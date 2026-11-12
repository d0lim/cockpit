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
| 8 | `ports` | an offset outside `0..99`, on the reserved offset 99, or taken twice; a port outside `1..65535`; a port on a service that the compose files (with their includes) do not define; a `databases[].port` that names no entry of `ports` |
| 9 | `compose` | a compose file that cannot be read or is not valid YAML; an `include` item or value that breaks the [include rules](#compose-includes), at its field (`compose.files[0] include[1].path[0]`); an include cycle; more than 64 compose file reads; a consumer service whose image uses the `cockpit-runner` repository, a builder or expected service that the compose files (with their includes) do not define, a consumer service named `cockpit-edge`, `cockpit-runner`, `cockpit-probe`, `cockpit-probe-control` or `cockpit-otlp`, or a network named `egress`; a `telemetry.otlp.services` entry that the compose files (with their includes) do not define or that names the builder |

## Compose includes

The model behind checks 8 and 9, the harness and the image keys holds every compose file compose reads, not only `compose.files`. The hub follows each `include:` through the commit's tree, depth first from each of `compose.files` in order, so includes into submodules work like any directory. It records the files it read: the declared files first, then each included file once, in the order it was first reached. An included file's services, volumes and networks join the model before those of the file that includes it, as compose merges them.

Paths resolve the way compose resolves them:

- A top-level `include:` in a declared file resolves against `compose.projectDirectory` (the repository root when omitted), because every compose call passes `--project-directory`. It does not resolve against the declared file's own directory.
- Every value of an include item (`path`, `project_directory`, `env_file`) resolves against the project directory of the file that holds the item.
- An included file's project directory is the item's `project_directory`, else the directory of the item's first `path`. That directory is the base of the included file's own `include:` items, build contexts and bind sources.

An item is a path string or a mapping of `path` (a string or a list), `project_directory` and `env_file` (a string or a list). Any other key is rejected at its field (`compose.files[0] include[1].profiles`). Every value:

- must not be empty, contain `$` (interpolation the hub cannot resolve), contain `:` (a remote include such as `oci://`, `https://` or `git@` makes compose fetch on the host when it renders), start with `~` (compose expands it to the hub user's home directory in `project_directory`), or be absolute;
- must resolve to a clean relative path inside the commit that passes through no committed symbolic link;
- as a `path`, must name a regular file of the commit whose name contains no entry of `compose.forbidden`; as a `project_directory`, a directory of the commit (a submodule root is one); as an `env_file`, a regular file of the commit or exactly one of `env.outputs`.

An included project without `env_file` reads the `.env` in its project directory. That file lies inside the confined project directory, and it must not be a committed symbolic link. It may be one of `env.outputs`, such as a submodule's generated `.env`. Compose applies these files as follows (observed with Compose v5.6.1 and `docker compose config`). The top project's own `.env` is never read, because every compose call passes `--env-file`. An included project still reads the `.env` of its project directory, and an include `env_file` replaces that `.env`. The variables the top project defines through its env files (`<compose.projectDirectory>/.env` when it is an output, then `env/run.env`) take precedence over an included project's `.env` or `env_file`, which only fill variables the top project leaves undefined. So `RUN_*`, the ports and the image keys reach included files unchanged.

An include that reaches a file already on its own include chain with the same project directory is a cycle. The same file reached through two chains is not a cycle, and compose accepts it; the hub reads it once for each chain. The model reads at most 64 compose files in all, counting each read.

`extends: { file: ... }` also makes compose read another file when it renders. The hub neither follows nor confines it; that gap predates includes and is open.

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
- **Image keys**: each key in `images` hashes the sorted git object ids of its inputs plus every compose file of the model, the declared files and every file they include (sha256, first 12 hex characters). An input that is a submodule root counts as its pinned commit id, and an input inside a submodule as its own object id. The runner image has no key; it belongs to the cockpit version.
- **run.env**: `RUN_ID`, `RUN_SRC`, `RUN_BUILD`, `RUN_WORK`, one `RUN_PORT_<NAME>` per port offset and one line per image key.

## Compose calls

Every compose call of a run names the project `-p <run id>` and `--project-directory src/<compose.projectDirectory>`, which is `src/` when the field is omitted. Compose resolves the relative paths of every file in the set (build contexts, bind sources) against that directory, not against the directory of the file that holds them, so a consumer whose compose files live in `backend/app/` declares `projectDirectory: backend/app` to keep `./server` and `../other` meaning what they mean there. The field must name a directory of the commit: confinement rejects an absolute path, a `..` segment, a path through a committed symbolic link, and a file or a missing directory. The env files follow: `src/<compose.projectDirectory>/.env` when that path is one of `env.outputs` (with the field omitted, the root `.env`), then `env/run.env`. Then come `-f` for each of `compose.files` in order and `harness.yaml` last. Included files get no `-f`: compose reads them through `include:`, and a top-level include path resolves against the project directory (see [Compose includes](#compose-includes)). The harness itself uses only absolute paths, so the project directory does not move it.

## Generated harness

`harness.yaml` is the last `-f` file after the declared `compose.files`, and the file set is refused when a declared name contains an entry of `compose.forbidden`; an included file with such a name rejects the declaration. It makes `default` and every non-external consumer network internal, adds `egress`, labels every service, network and volume of the model (included ones too) with `io.cockpit.managed` and `io.cockpit.run-id`, removes consumer `ports` with `!reset`, and moves the builder to `egress` only with `!override`. It adds four services on the runner image, and a fifth, `cockpit-otlp`, when `telemetry.otlp.services` names at least one service. Each is read-only, without capabilities and with memory and process limits:

| Service | Mode | Networks | Notes |
| --- | --- | --- | --- |
| `cockpit-edge` | `edge` | the internal networks and `egress` | publishes `127.0.0.1:<base + offset>` to container port `30000 + offset` and proxies to `<service>:<port>`; without declared `ports` it listens on nothing and runs until it is stopped |
| `cockpit-runner` | `run` (profile `runner`) | the internal networks | binds only `work/` paths; only `/work/e2e` is writable; its `environment` takes `E2E_EXEC_OUT` and `COCKPIT_TRACE_ID` from the compose process environment |
| `cockpit-probe` | `probe` (profile `probe`) | `default` | canary port `base + 99` |
| `cockpit-probe-control` | `probe` (profile `probe`) | `egress` | the positive control of the probe |
| `cockpit-otlp` | `otlp :4318` | the internal networks | the [OTLP receiver](#otlp-receiver); no ports, no binds and no profile, so it starts with the stack like the edge |

Every harness service name starts with `cockpit-`, so it never takes a name a consumer project would give its own service; a consumer service may be called `edge`, `runner`, `probe` or `otlp`. The mode is the `cockpit-runner` subcommand, and the subcommands and the profile names `runner` and `probe` keep their short names. Harness version 8 added the prefix: a kept run whose environment an older harness made carries the old names, and the hub refuses to address it ([Harness version](lifecycle.md#harness-version)).

For each service that `telemetry.otlp.services` names, the harness sets five variables in that service's `environment`:

| Variable | Value |
| --- | --- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://cockpit-otlp:4318` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` |
| `OTEL_METRIC_EXPORT_INTERVAL` | `2000` |
| `OTEL_BSP_SCHEDULE_DELAY` | `500` |
| `OTEL_TRACES_SAMPLER` | `parentbased_always_on` |

The harness file comes after the consumer files, so these keys win over the same keys in the consumer's `environment`, whether written as a map, a list or in an included file. Other services get none of them. The receiver is reachable only on the internal networks, and the seal assertions that confine every harness service (`services.ports`, `services.networks`, `mounts.binds`) confine it too.

On Linux the `cockpit-runner` service runs as the hub's uid:gid so it can write `work/e2e` (700) and read the private env (600) that the hub owns; a hub running as root keeps the image user `65532:65532`. On Docker Desktop (macOS) the file sharing layer maps ownership, so the image user stays.

Collector ids are the `.ts` basenames under `scripts.collectors`. They must match `^[a-z][a-z0-9-]{1,40}$`, and `runner`, `logs`, `services`, `supervisor`, `pipeline` and `otlp` are reserved for the hub's own signal sources. The secret sources and env outputs of the declaration are also treated as credential file names in the run record.

Each carries `io.cockpit.role` with its own service name as the value, and so does the builder (`builder`). Before any seal assertion, the rendered config (`docker compose config --format json` with every profile) must hold exactly one service per role (`cockpit-otlp` only when telemetry is declared), and every service must carry the labels of the run.

## SQL port

Each entry of `databases` names, in `port`, the offset whose edge port publishes that database. The hub reaches it at `127.0.0.1:<base + offset>` (`sqlport.TargetsFor`), the same loopback port a person would use, so the hub never joins a project network. The connection uses `rootUser` (driver default `root` for MySQL, `postgres` for PostgreSQL) with the secret named by `rootSecret` only to create a reader for this run, then reads as that reader. `database` names the database the reader connects to: for PostgreSQL the connection database (default `postgres`), and for MySQL the default database in which unqualified table names resolve. A MySQL declaration without it selects no database, so only qualified names (`app.items`) read, and a MySQL `database` outside `readSchemas` is refused with `PORT_DENIED` because the reader has no grant there.

`ctx.sql.query(db, statement, { params })` binds `params` to the placeholders of the statement (`?` for MySQL, `$1` to `$n` for PostgreSQL) through the driver: MySQL prepares the statement on the server, and PostgreSQL uses the extended protocol, so a value never becomes SQL text. The single-statement and read-only checks run on the statement as written, before any value is bound. Only strings, numbers, booleans and null are values: the collect module refuses `undefined` and holes, because JSON would turn them into null, and the port refuses objects, arrays and numbers JSON cannot carry, all with `PORT_BAD_ARGS` before the database is reached. More than 100 values, or more than 64 KiB counting each string's bytes and 8 for any other value, is `PORT_LIMIT`. A value count that does not match the placeholders is `PORT_BAD_ARGS`. Refusals name the index and the kind of a value, never the value.

## OTLP receiver

`cockpit-runner otlp <listen address>` receives OTLP/HTTP exports from the services of a run and prints what it receives as JSON lines on stdout. The harness runs it as the `cockpit-otlp` service. The code lives in `internal/runner/otlp`. It takes exactly one argument, a listen address such as `:4318`; a missing, extra or malformed argument, or an address it cannot listen on, exits 2. It serves until it receives SIGTERM or an interrupt, waits up to 5 s for requests in flight, and exits 0.

| Request | Answer |
| --- | --- |
| `POST /v1/traces`, `POST /v1/metrics` with `Content-Type: application/x-protobuf` or `application/json` | 200 with an empty export response in the request's encoding: an empty protobuf body, or `{}` |
| a body over 4 MiB, before or after `Content-Encoding: gzip` is undone | 413 |
| an export holding more than 50,000 spans or metric data points | 413 with the plain-text reason `export holds more than 50000 spans or data points` |
| a body that does not decode, or bad gzip | 400 |
| another `Content-Type`, or a `Content-Encoding` other than `gzip` or `identity` | 415 |
| another method on either path | 405 |
| any other path, `/v1/logs` included | 404 |
| an export whose lines cannot be written to stdout | 503; the lines of the chunks written before the failure stay |

A trace export decodes into `TracesData` and a metric export into `MetricsData` of `go.opentelemetry.io/proto/otlp`. Both hold the resource list as field 1, the same wire layout as the collector's export requests, so the runner links no grpc code; a build test fails when `go version -m` on the runner shows `google.golang.org/grpc`. OTLP/JSON writes `traceId`, `spanId` and `parentSpanId` as hex, so before the JSON decode each such key whose value is 32 or 16 hex characters is rewritten to base64, wherever it appears (span links and exemplars included). A plain `protojson` decode would read 32 hex characters as base64 and return 24 wrong bytes without an error.

The receiver handles one export at a time: after a request's body is read, it waits until the export before it is written, so the memory of the receiver is one decoded export plus one chunk of lines, however many services export at once. A decoded 4 MiB body can still take about 600 MiB, because an empty span costs 2 bytes on the wire and about 300 bytes decoded, and the serial handling keeps that cost to one export at a time. The receiver also sets a soft memory limit of 256 MiB unless the environment sets `GOMEMLIMIT`, so the garbage of one export is collected before the next export grows the heap: without it, five such bodies in a row held the container at its 1 GiB limit. It counts the spans of a trace export, or the data points of a metric export, before it builds any line and refuses more than 50,000 with 413. An SDK batch stays far below that: the batch span processor exports at most 512 spans by default, and a metric export holds one point per series, which the OpenTelemetry specification limits to 2,000 per instrument by default. It then builds the lines of an accepted export in chunks of 1,024 signals, spread over the CPUs, and writes each chunk to stdout before it builds the next, so the lines of an export keep their order, the lines of two exports never interleave, and a chunk is released before the next one is built.

Each output line is `{"v":1,"signal":{...}}`. The signal holds the fields the hub appends to the run's stream (`docs/signals.md`): `kind`, `id`, `rev`, `ts`, `tsSource`, `attrs` and a `span` or `metric` body. The hub sets the collector and the sequence number. `tsSource` is always `container`, and `ts` is RFC 3339 with nanoseconds in UTC. Every attribute carries the resource's `service.name` as `service`, which wins over a span or point attribute of that name.

- **Span**: one line per span. `id` is `w3c:<span id>`, `span.traceId` is `w3c:<trace id>` and `span.parentId` is `w3c:<parent span id>` (absent on a root span), all in lowercase hex. `ts` and `span.start` are the start time, `span.end` the end time, `span.status` is `error` when the OTLP status code is `ERROR` and `ok` otherwise, and each span link becomes a `span.links` entry with the same `w3c:` form. `attrs` are the span attributes plus `service`.
- **Metric**: one line per data point, with `metric.name`, `metric.unit` and `metric.type` (`gauge`, `sum`, `histogram` or `summary`). `ts` is the point time. `attrs` are the point attributes plus `service`. A gauge, a cumulative sum, a cumulative histogram and a summary are series: `id` is `metric:` plus the first 16 bytes, in hex, of SHA-256 over the service, the metric name and the point attributes sorted by key, and `rev` is the point time in Unix milliseconds. The point time keeps rising when the receiver restarts, so the newest total wins without state in the receiver, where a counter would start again at 1 and lose. A sum or histogram point of any other temporality (delta, or unspecified) carries no `id` and no `rev`, so each point stays its own record, because a delta total cannot be rebuilt after a restart. A histogram's `value` is its sum and its `histogram` holds `count`, `sum`, `bounds` and `counts`; when the counts are not one more than the bounds or the bounds do not ascend, only `count` and `sum` stay. An exponential histogram keeps only `count` and `sum` as a `histogram`. A summary's `value` is its sum, and its count is the attribute `count`. A point whose value or sum is not a finite number is dropped and logged, because JSON cannot carry it. A histogram point whose bounds hold a number that is not finite keeps `count` and `sum` without `bounds` and `counts`, and that is logged too.

Every string of a signal passes the runner redactor (`mask.Runner`) before it is cut. The receiver has no private env file, so only the pattern rules apply, and an attribute whose key names a credential (`authorization`, `cookie`, a key ending in `token` or `secret`, and the others `docs/signals.md` lists) reads `****` whole. The hub masks again with the run's value list when it appends. After redaction every string is cut to at most 1 KiB at a character boundary. A line stays under 64 KiB: when it would not, the receiver drops, in order, the attributes other than `service` (and sets `attrsDropped` to their number), the span links, and the histogram buckets, until it fits.

Diagnostics (a refused request, a dropped point, dropped buckets, a write failure) go to stderr as `log/slog` JSON records that name the path, the status and counts, never a received value.
