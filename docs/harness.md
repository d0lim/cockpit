# Declaration and harness

This page holds the rules the hub follows between reading a commit's declaration and handing compose a file set. The code lives in `internal/compose`; the hub settings live in `internal/hub/config`.

## Reading the declaration

The hub reads `.cockpit/project.yaml` from the git object of the submitted commit (`git cat-file`), never from the work tree. The commit's tree includes every pinned submodule commit, recursively, under its path (see [Git](#git)), so declared paths cross into submodules as if they were directories. The checks run in this order, and the first failure rejects the run with its kind and the field it concerns:

| Order | Kind | Rejects |
| --- | --- | --- |
| 1 | `submodule` | a gitlink whose path is not a clean relative path, that has no entry in its repository's `.gitmodules` or is claimed by two; a `.gitmodules` that is a link, not a file or not git config; a submodule name that is not a clean relative path or resolves outside `modules/`; a submodule not initialized in the main repository; a pinned commit its repository does not hold; more than 64 submodules in one commit |
| 2 | `missing` | a commit without the file |
| 3 | `unreadable` | a link, a non-file, a file over 1 MiB, or invalid YAML |
| 4 | `schema` | a document that fails `schema/declaration.json` or carries unknown fields |
| 5 | `confinement` | a path that is not a clean relative path, passes through a committed symbolic link, or is missing where a file or directory is required (a submodule root is a directory); a name outside `^[a-z0-9][a-z0-9-]{0,62}$` or used twice; suites or collectors outside `scripts.dir` |
| 6 | `host-api` | a `hostApi` outside the set this hub serves; the message names the served set and the cockpit version |
| 7 | `secrets` | an empty `secrets.sources` |
| 8 | `ports` | an offset outside `0..99`, on the reserved offset 99, or taken twice; a port outside `1..65535`; a port on a service that `compose.files` does not define; a `databases[].port` that names no entry of `ports` |
| 9 | `compose` | a consumer service whose image uses the `cockpit-runner` repository, a builder or expected service that `compose.files` does not define, or a consumer service named `edge`, `runner`, `probe` or `probe-control`, or a network named `egress` |

## Allowed repositories

`<COCKPIT_HOME>/config.yaml` lists `allowedRepositories` as absolute paths. A submission passes when the real path of its `git rev-parse --git-common-dir` equals that of a listed entry, so linked worktrees and subdirectories of a listed repository pass and a repository nested inside one does not. The same file overrides the core limits: `limits.keptPool` (5), `limits.runningCap` (3), `limits.queueCap` (10), `ports.base` (22000) and `ports.slots` (8). Each must be a positive integer, the port block starts at 1024 or above, and its last port is at most 65535. `reservedPorts` lists port ranges the hub must never lease, each with a unique `name` and the half-open range `[from, to)`: `from` and `to` lie in `1..65535` and `from < to`, so `{ name: old-hub, from: 21000, to: 21800 }` reserves 21000 through 21799. Ports that belong to a consumer project live only here, never in core. A missing file means the defaults, no allowed repository and no reserved range.

## Git

Every git call of the hub passes `-c core.fsmonitor=false`, `-c core.hooksPath=<empty temporary directory>`, `-c maintenance.auto=false`, `-c maintenance.autoDetach=false`, `-c gc.auto=0`, `-c gc.autoDetach=false` and `--no-optional-locks`, and runs without inherited `GIT_*` variables. The maintenance and gc settings matter because git commands such as `commit` start `git maintenance run --auto --detach`, a child that leaves the call and writes `objects/maintenance.lock` into the repository afterwards; the hub must not leave a git process writing into a consumer repository. Tests that build repositories with plain git set the same four settings through `GIT_CONFIG_COUNT`. Commit input resolves through `<ref>^{commit}` to a full sha before any other call uses it.

A gitlink in the commit stands for the pinned commit of its submodule, and the hub reads that commit as part of the run. The submodule's repository is `<git common dir>/modules/<name>`, and a nested one lies under its parent's (`<git common dir>/modules/<a>/modules/<b>`), which is where git keeps them for the main checkout. The name comes from `submodule.<name>.path` in the `.gitmodules` blob of the same commit, read with `git config -z --no-includes --blob <id> --list`, so an `[include]` in a committed `.gitmodules` is never followed. The name must be a clean relative path, and it is resolved through `os.Root` on `modules/`, so `..`, an absolute name or a link cannot lead out of it. Object reads (`ls-tree`, `cat-file`, `archive`, `config --blob`) run in the git directory with `--git-dir=. --work-tree=.`, so a submodule's `core.worktree` is never followed and the objects stay readable after its checkout is removed. The hub never fetches, never runs `git submodule`, and never reads a linked worktree's `worktrees/<name>/modules/`, so submodules must be initialized in the main checkout (`git submodule update --init --recursive`). Every gitlink must resolve, including one that no declared path touches, and a commit may pin at most 64 submodules in all.

The dirty check runs `git status --porcelain -z --untracked-files=normal --ignore-submodules=dirty` in the worktree and in each checked-out submodule that a declared path lies in or contains, each with the declared paths relative to that checkout. Each such submodule's path is added to its parent's status, so a submodule checked out at a commit other than the one its parent records is dirty whatever `ignore=` in `.gitmodules` says; changes inside a submodule come from the hub's own status in that submodule. A submodule whose directory is missing or empty is not checked out and has nothing uncommitted; a non-empty directory that is not its own checkout is dirty.

## Run directory

The hub opens each run directory with `os.Root`. It holds `src/` and `build/` (two extractions of the commit and of every pinned submodule commit under its path, each repository's archive checked against its file count in the tree), `work/` (the runner's view), `env/` (`run.env` and the kit README) and `harness.yaml`. Directories the hub creates for credentials are 700 and their files 600.

- **Environment generation**: `env.command` runs in `docker run --rm --network none` with the digest-pinned `env.image`, a non-root user, a read-only root, all capabilities dropped, only `src/` bound at `/src`, and only `COMPOSE_PROJECT_NAME` in its environment, under `umask 077`. Every output must then be a regular file, and the hub sets it to 600. A failure removes the outputs that did not exist before and reports the exit code with the masked end of stderr. Every output is a value source of the run redactor; `secrets.sources` adds more.
- **Work tree**: `work/scripts` copies `scripts.dir`, `work/.cockpit/project.yaml` copies the declaration, and `work/source/<name>` copies each `inputs.runnerSources` entry. A symbolic link inside a copied tree fails the copy. `work/e2e/e2e.private.env.json` is the env output of that name, or `{"local": {}}` when none is declared.
- **Image keys**: each key in `images` hashes the sorted git object ids of its inputs plus the declared compose files (sha256, first 12 hex characters). An input that is a submodule root counts as its pinned commit id, and an input inside a submodule as its own object id. The runner image has no key; it belongs to the cockpit version.
- **run.env**: `RUN_ID`, `RUN_SRC`, `RUN_BUILD`, `RUN_WORK`, one `RUN_PORT_<NAME>` per port offset and one line per image key.

## Compose calls

Every compose call of a run names the project `-p <run id>` and `--project-directory src/<compose.projectDirectory>`, which is `src/` when the field is omitted. Compose resolves the relative paths of every file in the set (build contexts, bind sources) against that directory, not against the directory of the file that holds them, so a consumer whose compose files live in `backend/app/` declares `projectDirectory: backend/app` to keep `./server` and `../other` meaning what they mean there. The field must name a directory of the commit: confinement rejects an absolute path, a `..` segment, a path through a committed symbolic link, and a file or a missing directory. The env files follow: `src/<compose.projectDirectory>/.env` when that path is one of `env.outputs` (with the field omitted, the root `.env`), then `env/run.env`. Then come `-f` for each of `compose.files` in order and `harness.yaml` last. The harness itself uses only absolute paths, so the project directory does not move it.

## Generated harness

`harness.yaml` is the last `-f` file after the declared `compose.files`, and the file set is refused when a name contains an entry of `compose.forbidden`. It makes `default` and every non-external consumer network internal, adds `egress`, labels every service, network and volume with `io.cockpit.managed` and `io.cockpit.run-id`, removes consumer `ports` with `!reset`, and moves the builder to `egress` only with `!override`. It adds four services on the runner image, each read-only, without capabilities and with memory and process limits:

| Service | Mode | Networks | Notes |
| --- | --- | --- | --- |
| `edge` | `edge` | the internal networks and `egress` | publishes `127.0.0.1:<base + offset>` to container port `30000 + offset` and proxies to `<service>:<port>`; without declared `ports` it listens on nothing and runs until it is stopped |
| `runner` | `run` (profile `runner`) | the internal networks | binds only `work/` paths; only `/work/e2e` is writable |
| `probe` | `probe` (profile `probe`) | `default` | canary port `base + 99` |
| `probe-control` | `probe` (profile `probe`) | `egress` | the positive control of the probe |

On Linux the `runner` service runs as the hub's uid:gid so it can write `work/e2e` (700) and read the private env (600) that the hub owns; a hub running as root keeps the image user `65532:65532`. On Docker Desktop (macOS) the file sharing layer maps ownership, so the image user stays.

Collector ids are the `.ts` basenames under `scripts.collectors`. They must match `^[a-z][a-z0-9-]{1,40}$`, and `runner`, `logs`, `services` and `supervisor` are reserved for the hub's own signal sources. The secret sources and env outputs of the declaration are also treated as credential file names in the run record.

Each carries `io.cockpit.role`, and so does the builder (`builder`). Before any seal assertion, the rendered config (`docker compose config --format json` with every profile) must hold exactly one service per role, and every service must carry the labels of the run.

## SQL port

Each entry of `databases` names, in `port`, the offset whose edge port publishes that database. The hub reaches it at `127.0.0.1:<base + offset>` (`sqlport.TargetsFor`), the same loopback port a person would use, so the hub never joins a project network. The connection uses `rootUser` (driver default `root` for MySQL, `postgres` for PostgreSQL) with the secret named by `rootSecret` only to create a reader for this run, then reads as that reader. `database` names the database the reader connects to: for PostgreSQL the connection database (default `postgres`), and for MySQL the default database in which unqualified table names resolve. A MySQL declaration without it selects no database, so only qualified names (`app.items`) read, and a MySQL `database` outside `readSchemas` is refused with `PORT_DENIED` because the reader has no grant there.

`ctx.sql.query(db, statement, { params })` binds `params` to the placeholders of the statement (`?` for MySQL, `$1` to `$n` for PostgreSQL) through the driver: MySQL prepares the statement on the server, and PostgreSQL uses the extended protocol, so a value never becomes SQL text. The single-statement and read-only checks run on the statement as written, before any value is bound. Only strings, numbers, booleans and null are values: the collect module refuses `undefined` and holes, because JSON would turn them into null, and the port refuses objects, arrays and numbers JSON cannot carry, all with `PORT_BAD_ARGS` before the database is reached. More than 100 values, or more than 64 KiB counting each string's bytes and 8 for any other value, is `PORT_LIMIT`. A value count that does not match the placeholders is `PORT_BAD_ARGS`. Refusals name the index and the kind of a value, never the value.
