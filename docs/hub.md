# Running on the hub

This guide takes a compose project to its first sealed run on the hub. The [usage guide](guide.md) covers writing suites and running the runner against a stack you start yourself; everything there about suites, `ctx` and the helpers applies here too. The run rules themselves are in [harness](harness.md), [sealing](sealing.md), [lifecycle](lifecycle.md) and [verdict](verdict.md); this page says what a consumer writes and why.

A sealed run is one commit and one suite on a fresh environment: the hub copies the commit out of git, generates credentials inside the run, starts the stack on internal networks with no route out, runs the suite in a container on those networks, and reads the verdict from the runner's result file. Uncommitted changes are never part of a run.

## Steps

1. Start a hub and allow the repository.
2. Write `.cockpit/project.yaml`.
3. Generate the run's credentials with `env.command`.
4. Replace every external service with a fake on the stack.
5. Give each expected service a readiness signal.
6. Write a suite, and optionally collectors.
7. Commit, then `cockpit runs submit --wait`.

## 1. Start a hub

The hub keeps its state in `COCKPIT_HOME` (default `~/.local/state/cockpit`) and listens on `127.0.0.1:7710` (`COCKPIT_PORT`). It runs only commits of the repositories listed in `<COCKPIT_HOME>/config.yaml`:

```yaml
allowedRepositories:
  - /Users/me/src/shop
```

The path is compared after resolving links, against the repository's git common directory, so worktrees of a listed repository pass. On macOS `/tmp` is `/private/tmp`; either spelling works because both resolve the same way. The same file holds the hub limits; the keys and defaults are in [harness](harness.md#allowed-repositories). Then:

```sh
cockpit hub
```

The screen is at `http://127.0.0.1:7710`. `cockpit runs` reads the hub address from `COCKPIT_URL` (default `http://127.0.0.1:7710`), not from `COCKPIT_PORT`.

### A second hub on the same host

Each hub leases its own port block, and the defaults are the same for every hub, so a second hub needs all of these:

| Setting | Why |
| --- | --- |
| `COCKPIT_HOME` | its own state directory |
| `COCKPIT_PORT` | its own listen port; set `COCKPIT_URL` to match for `cockpit runs` |
| `ports.base` in its `config.yaml` | a block that does not overlap the other hub's (`base` to `base + slots * 100`) |
| `reservedPorts` in its `config.yaml` | the other hub's block, so this hub never leases into it |

Hubs register in a shared registry (`~/.local/state/cockpit-hubs`, moved with `COCKPIT_REGISTRY`) so each screen shows the others and knows whose environments it sees. A hub with its own registry works, but it lists the other hub's environments as orphans. The running cap counts every running compose project on the host that is not this hub's, so another hub's kept environments use up this hub's cap.

## 2. The project declaration

The hub reads `.cockpit/project.yaml` from the submitted commit. `schema/declaration.json` describes every field; this is a complete declaration for a project with an API, a worker, a PostgreSQL database and a fake for an external service:

```yaml
hostApi: 1
project:
  expectedServices: [db, api, worker, rates]
compose:
  files: [compose.yaml, .cockpit/compose.e2e.yaml]
env:
  image: alpine@sha256:<digest>
  command: [sh, .cockpit/gen-env.sh]
  outputs: [.env, e2e.private.env.json]
secrets:
  sources:
    - { path: .env, format: dotenv }
ports:
  - { name: api, offset: 0, service: api, port: 8080, display: Shop API, open: link }
  - { name: db, offset: 1, service: db, port: 5432, display: PostgreSQL, open: copy }
databases:
  - name: shop
    driver: postgres
    port: db
    readSchemas: [public]
    rootSecret: DB_PASSWORD
    rootUser: shop
    database: shop
scripts:
  dir: e2e
  suites: e2e/suites
  collectors: e2e/collectors
```

For MySQL the database entry differs only in what the names mean: `readSchemas` lists databases, `database` (optional) is the default database for unqualified table names and must be one of them, and `rootUser` defaults to `root`:

```yaml
databases:
  - { name: library, driver: mysql, port: mysql, readSchemas: [library], database: library, rootSecret: MYSQL_ROOT_PASSWORD }
```

Every path is relative to the repository root. The fields an adopter usually gets wrong:

- **`project.expectedServices`** are the services the boot stage waits for. Leave out services that only run under a profile.
- **`compose.files`** is the whole file set, in order. A consumer keeps its own `compose.yaml` and adds an overlay for what only the sealed run needs (fakes, healthchecks, env overrides). Do not add the runner overlay that `cockpit init` writes: the hub generates its own harness, and it removes every published `ports:` entry of the consumer.
- **`ports`** publishes a service on the host. Offset `n` is reachable at `127.0.0.1:<base + n>` while the run's environment is up, and its name is how collectors reach it (`ctx.http`, `databases[].port`). Offset 99 is reserved for the seal probe.
- **`databases`** lets collectors read a database through `ctx.sql.query(name, ...)`, where `name` is `databases[].name`. The database must also be published under `ports`. `rootSecret` is a key in a `dotenv` source of `secrets.sources`; the hub logs in as `rootUser` (default `postgres` or `root`) with that value only to create a read-only user for the run, and reads as that user. For PostgreSQL `readSchemas` are schemas inside `database` (default `postgres`), and the reader can select from the tables that exist when the collectors start; name the role that creates later tables in `ownerRole`. For MySQL `readSchemas` are databases.
- **`scripts`**: suites are `<scripts.suites>/<name>.ts`, collectors `<scripts.collectors>/<id>.ts`, and both directories sit inside `scripts.dir`, which is what the runner sees as `/work/scripts`. `env.json` lives at the top of `scripts.dir`.

The hub checks the declaration in a fixed order and rejects a submit with the first problem's kind and field; the table is in [harness](harness.md#reading-the-declaration).

## 3. Credentials

Credentials never come from the repository. `env.command` runs once per run, before compose starts, in `env.image` with no network, as the hub's user, with the extracted commit mounted writable at `/src` as the working directory. The image must be pinned by digest (`docker buildx imagetools inspect alpine:3.20` prints it) and must have `/bin/sh`. The command writes each of `env.outputs` under `/src`, and the hub makes them mode 600:

```sh
#!/bin/sh
set -eu
rand() { head -c 24 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 24; }
db=$(rand)
admin=$(rand)
printf 'DB_PASSWORD=%s\nADMIN_TOKEN=%s\n' "$db" "$admin" > .env
printf '{"local":{"adminToken":"%s"}}\n' "$admin" > e2e.private.env.json
```

- `.env` at the compose project directory, when it is an output, is the env file of every compose call, so `${DB_PASSWORD}` in `compose.yaml` resolves to the run's value.
- An output named `e2e.private.env.json` becomes the runner's private env, which flows read as `ctx.priv`.
- Every output and every `secrets.sources` entry feeds the run redactor: their values are masked as `****` everywhere the run leaves text. `secrets.publicNames` lists dotenv keys or JSON field names whose values are not secret (a user name, a region) and stay readable; `secrets.patterns` adds regular expressions to mask.

## 4. External services

At runtime the long-running services have no route out of the run: the hub makes every consumer network internal and the seal check proves it. A service that calls an outside API fails inside a sealed run. Point it at a fake on the stack instead, in the overlay:

```yaml
services:
  rates:
    build: ./e2e/fakes/rates
  api:
    environment:
      RATES_URL: http://rates:8080
```

Building is the exception: `compose build` runs with network access, so a Dockerfile may download dependencies. Downloads that do not fit a Dockerfile (vendoring dependencies into the source before the build) can run in the declared `project.builder`, a service that runs in the build stage, alone on the outside network, binding only the run's build copy of the commit; see [sealing](sealing.md).

## 5. Readiness

The boot stage waits until each expected service runs and, when it has a healthcheck, is healthy. A service without a healthcheck counts as ready as soon as its container starts, so the suite can race the server's start. Give every expected HTTP service a healthcheck, and gate dependents with `depends_on: {condition: service_healthy}`. A one-shot service (a migration) is ready when it exits 0.

## 6. Suite and collectors

The suite is written exactly as in the [usage guide](guide.md#writing-a-suite). `env.json` names the services by their compose names (`"baseUrl": "http://api:8080"`), because the runner joins the run's internal networks. Mark each id a flow creates with `ctx.ref` so the screen can relate rows and log lines to the step that made them ([guide](guide.md#correlation-keys-and-request-records)).

### Collectors

A collector is a script the hub runs while the environment is up, to record what happened inside it: database rows, log lines, queue contents. It never affects the verdict: the report shows each collector as a stage of its own, `ok` or `error`. Each file `<scripts.collectors>/<id>.ts` default-exports `collector({ id, intervalMs?, run })`, and `id` must equal the file name:

```ts
import { collector } from "cockpit:collect";

export default collector({
	id: "orders-db",
	intervalMs: 2000,
	async run(ctx) {
		const orders = await ctx.sql.query("shop", "select id, status from orders order by created_at");
		for (const [id, status] of orders.rows) {
			ctx.signal.event({
				id: `order:${id}`,
				attrs: { table: "orders", "ref.order": String(id), status: String(status) },
				body: JSON.stringify({ id, status }),
			});
		}
	},
});
```

- **When it runs**: first when the environment is ready, alongside the suite, then every `intervalMs` (1000 to 3600000) while the environment is up, and once more when the report is written. Without `intervalMs` it runs only at the report.
- **Revisions**: an event with an `id` that is emitted again replaces the earlier one on reads, and an unchanged row is not stored twice, so emitting every row on every run is the intended pattern. Add `rev` to order revisions explicitly.
- **Attributes the screen reads**: `table` groups events into one table per name in the Table view, `status` and `tone` color the row, `label` titles it, and the keys of a JSON `body` become table columns. A `ref.<name>` attribute relates the row to the steps that marked the same id; declare only the ids that identify the row and keep parent ids in the body, as [correlation keys](guide.md#correlation-keys-and-request-records) explains.
- **Ports**: `ctx.sql`, `ctx.http` (GET and HEAD to the declared port names), `ctx.docker` (logs, service states, and `exec` of argv prefixes listed in `exec.allow`) and `ctx.state` for a cursor between runs of the collector. The declarations in `@d0lim/cockpit-sdk` document each one.
- **Where to see it**: the run's Signals screen lists each collector with its state and failure count, and `GET /api/runs/<id>/signals` returns the same under `collectors`. A passed verdict says nothing about collectors.

## 7. Submit and read the result

Commit first: the hub reads only the commit, and it refuses a submit when a path listed in `inputs.dirty` has uncommitted changes.

```sh
cockpit runs submit --wait
```

`submit` sends the repository of the current directory at `HEAD` (or `--commit`), and with one suite in the commit picks it; otherwise name it with `--suite`. The exit code follows the verdict (`0` passed, `1` failed, `2` error; the full table is in [api](api.md#cli)). Then:

- `cockpit runs report <id>` prints the flows and steps, and the screen at `/runs/<id>` shows the report, each flow's steps with their requests and refs, and the signals.
- When the run ends its containers stop, and the environment stays in the kept pool. `cockpit runs resume <id>` brings it back, `cockpit runs exec <id> -- <flow>` reruns one flow against it, and `cockpit runs destroy <id>` removes it. The report bundle stays after a destroy.

## Common refusals

| Refusal | Fix |
| --- | --- |
| the repository is not allowed | add its path to `allowedRepositories` and restart the hub |
| `dirty` | commit the listed files |
| `missing`, `schema`, `confinement`, `host-api`, `ports`, `compose` | the field the message names; see [harness](harness.md#reading-the-declaration) |
| a suite name is needed | pass `--suite`; the message lists the commit's suites |
| the seal check fails | a service reaches outside the run or binds a path outside it; the report's seal result names the assertion ([sealing](sealing.md)) |
| boot fails | the notice names the service that exited non-zero, died or stayed unhealthy, and the run's tail holds the last 40 lines of its log, masked: the run's page shows the tail, and `cockpit runs report <id>` prints its last lines. The containers stay stopped until the run is destroyed |
