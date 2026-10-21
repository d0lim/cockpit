# Usage guide

This guide covers running suites with the cockpit runner against a compose stack you start yourself. The README's quick start is the shortest working setup; this page explains each part of it. To run a suite as a sealed run of a commit on the hub, read [Running on the hub](hub.md) next: everything here about suites and `ctx` applies there too.

`cockpit init [--base-url <url>] [<dir>]` writes that setup: the declaration, `e2e/env.json` with `baseUrl`, an example suite, a tsconfig, the compose overlay `compose.cockpit.yaml` and the state directory `.e2e/` with an empty private env. It refuses to overwrite any existing file, and it rejects a base URL that the runner would not accept as an origin.

## What goes where

The runner reads everything from fixed paths inside its container. A compose overlay mounts your files there.

| Container path | Holds | Mount |
| --- | --- | --- |
| `/work/scripts` | `env.json`, `suites/`, and the files the suites import | read-only |
| `/work/.cockpit/project.yaml` | the project declaration | read-only |
| `/work/e2e` | run state: ledger, context, results, and by default the private env | writable |
| `/work/source/<name>` | read-only directories that flows read with `ctx.readSource` | read-only, optional |

Each path can be moved with an environment variable on the runner service:

| Variable | Default |
| --- | --- |
| `COCKPIT_SCRIPTS` | `/work/scripts` |
| `COCKPIT_DECLARATION` | `/work/.cockpit/project.yaml` |
| `E2E_DIR` | `/work/e2e` |
| `E2E_ENV` | `$COCKPIT_SCRIPTS/env.json` |
| `E2E_PRIVATE_ENV` | `$E2E_DIR/e2e.private.env.json` |
| `COCKPIT_SOURCES` | `/work/source` |
| `E2E_EXEC_OUT` | a timestamped file under `$E2E_DIR/exec/` |

The runner image runs as uid `65532`. On Linux, make the state directory writable for that user, for example with `chmod 0777 .e2e`. Docker Desktop on macOS needs nothing extra.

## The project declaration

`.cockpit/project.yaml` must exist and must declare the host API the suite was written against:

```yaml
hostApi: 1
```

The runner exits with code 2 when the file is missing, has no `hostApi`, or names a different host API than the runner serves. The remaining declaration fields (compose files, services, secrets, port slots) belong to the sealed-run hub and are ignored by the runner; [Running on the hub](hub.md) explains them.

## Public env and the request allowlist

`env.json` is one JSON object. Flows read it as `ctx.env`.

```json
{
  "baseUrl": "http://web",
  "adminUrl": "http://admin:8080"
}
```

The same file is the request allowlist. Every string value that is an `http` or `https` URL with a host name, no IP address and no path becomes an allowed origin. `ctx.http` refuses a request to any other origin. Name your services in compose and put their base URLs here; a URL that carries a path, such as `http://web/api`, is a value but not an origin. A refused request fails its step with `address not allowed — GET /path: list its origin ... as a value in env.json`.

Other limits of `ctx.http`: 30 seconds per request unless `timeoutMs` says otherwise, response bodies up to 8 MiB, and redirect responses are refused instead of followed.

## Private env

The private env holds credentials. Flows read it as `ctx.priv`, and every value in it is masked as `****` in the console, in failure messages and in the output files.

```json
{
  "local": {
    "adminPassword": "change-me"
  }
}
```

Only the `local` section is read. The file must exist even when the section is empty. Keep it out of the scripts directory and out of version control.

## Writing a suite

A suite file lives at `suites/<name>.ts` and default-exports `suite({ ... })`. The `name` must equal the file name, start with a lowercase letter, and use only lowercase letters, digits and hyphens.

- **Stages** group flows for reading the results. Every flow names one stage.
- **Flows** are an ordered list of steps that reach one goal. A flow id is lowercase letters, digits and hyphens, two to forty-one characters.
- **Steps** are one request, one wait or one capture (`kind: "request" | "wait" | "capture"`). A step fails when its `run` throws.
- **`requires`** lists flows that must pass first. When a flow fails, its remaining steps and every flow that requires it are skipped. A skip is not a pass.
- **`once: true`** marks a flow that may run only once per environment, such as creating the first admin. The ledger remembers it.

Suites import only from `cockpit:flow` and from relative paths inside the scripts directory. The runner bundles the suite with its imports and runs it in an embedded JavaScript engine, so there is no `node:*`, no `fetch`, no npm package and no file system. Everything a flow touches goes through `ctx`.

## The `ctx` ports

| Member | Does |
| --- | --- |
| `env`, `priv` | the public env and the private env, read-only |
| `http.send(method, url, opts?)` | sends one request to an allowed origin; `opts` takes `headers`, `json`, `body`, `timeoutMs` |
| `capture(name, value, { persist? })` | stores a value for later steps; `persist: true` keeps it in the context file across runs |
| `ref(name, value)` | marks `value` as a public correlation key of the running step, under `name`; see below |
| `get(name)` | reads a captured value and fails the step when it is missing |
| `totp(loginId)`, `saveSeed(loginId, secret)` | produces a TOTP code from a stored seed, and stores a seed into the private env |
| `crypto` | `randomUUID`, `getRandomValues` and RSA signing through `subtle` |
| `readSource(source, path)`, `listSource(source, dir)` | reads files under `/work/source/<source>` |
| `clock()`, `sleep(ms)` | the current time in milliseconds and a pause |

The engine also provides `Buffer`, `TextEncoder`, `URL`, `URLSearchParams`, `atob`, `btoa`, `setTimeout` and `console`. Console output goes to the runner's stderr, masked.

## Correlation keys and request records

Captured values are treated as secrets: the runner masks every occurrence of them as `****` in its output, the result file and the step messages. Ids that a flow creates (a transaction id, a reference id) are usually captured, and once masked nothing that the run leaves behind can say which request made which row.

`ctx.ref(name, value)` says that `value` is a public id. From then on the runner no longer masks it, even when it is also captured, and it records `{name, value}` on the running step. The name matches `^[a-z][A-Za-z0-9]{0,31}$`; the value is 1 to 200 characters without control characters. A value that the runner masker would change using only the private env values and its credential patterns is refused with `PORT_BAD_ARGS`, and stays masked: a private value or one that contains it (private values shorter than 6 characters are not masked, so they block nothing), or a credential shape such as a bearer token or a JWT. A step keeps at most 50 refs; marking the same name and value again in one step is a no-op, so a ref inside a polling loop records once.

```ts
const tx = json<{ id: string }>(await ctx.http.send("POST", `${ctx.env.baseUrl}/transfers`, { json: body }));
ctx.capture("transferId", tx.id);
ctx.ref("transaction", tx.id);
```

The runner also records every request a step sends: method, host, path (never the query or a body), status, duration and start time, at most 50 per step. Paths pass the runner masker, so an id appears in a path only after the flow marked it with `ref`.

The hub turns refs into attributes `ref.<name>` of the step's span and each request into a span under it (`docs/signals.md`). Collectors that put the same values in their own `ref.<name>` attributes let the screen link a database row, a notification or a log line back to the step and request that caused it.

## Helpers

`cockpit:flow` exports four helpers next to `suite`, `stage`, `flow` and `step`:

- `expectStatus(res, ...expected)` throws unless the status is one of `expected`. The message names the method and path, never the query or the body.
- `json<T>(res)` parses the body and throws without the body when it is not JSON.
- `target(method, url)` formats `GET /path` for your own messages.
- `poll(attempt, done, { what, deadlineMs, intervalMs, now, sleep })` repeats `attempt` until `done` holds and returns `{ value, attempts }`. It throws once the next attempt would pass the deadline. Pass `ctx.clock` and `ctx.sleep` as `now` and `sleep`.

```ts
const { value } = await poll(
	async () => json<{ state: string }>(await ctx.http.send("GET", `${ctx.env.baseUrl}/jobs/${id}`)).state,
	(state) => state !== "PENDING",
	{ what: "job state", deadlineMs: 30_000, intervalMs: 500, now: ctx.clock, sleep: ctx.sleep },
);
```

## Running

```sh
docker compose -f compose.yaml -f compose.cockpit.yaml run --rm runner run <suite>
docker compose -f compose.yaml -f compose.cockpit.yaml run --rm runner run <suite> --flow <id> [--flow <id>]...
```

A full run executes every flow in order and writes `manifest.json` when it starts and `result.json` as each step finishes. It does not consult the ledger before starting, so run it against a fresh environment: a new compose project and an empty state directory.

`--flow` runs only the named flows against the environment as it is. Repeatable requires run again first. A `once` require that already passed is not rerun; its captures are reused from the context. A `once` target that already ran is refused. The results go to one file per target under `exec/` instead of `result.json`.

To start over, bring the stack down with its volumes and delete the state directory except the private env.

## Output files

| File | Content |
| --- | --- |
| `manifest.json` | the suite's stages, flows and steps, with source file and line |
| `result.json` | one entry per step: status, duration, request count, last HTTP status, failure message |
| `ledger.json` | the last status of every flow in this environment |
| `context.json` | persisted captures and TOTP steps |
| `exec/<time>-<flow>.json` | the result of a `--flow` target |

Step statuses are `passed`, `failed`, `skipped`, `running` and `notRun`. The files are rewritten atomically, so a reader never sees a half-written file.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | every step passed |
| 1 | a step failed |
| 2 | the run was refused before it started: bad arguments, a missing or mismatched declaration, an unreadable env file, a suite that does not load or bundle, or a `once` flow that already ran |

## Type checking

The declarations for `cockpit:flow`, the engine globals and a tsconfig base come from the npm package `@d0lim/cockpit-sdk`, or from `cockpit sdk write <dir>` when no registry is available. Extend the base from the scripts directory:

```json
{
  "extends": "@d0lim/cockpit-sdk/tsconfig.base.json",
  "include": ["suites/*.ts"]
}
```

A project that keeps its own tsconfig can set `"types": ["@d0lim/cockpit-sdk"]` instead. Editors pick up the same project, so hover shows each port's documentation and errors appear as you type. `tsc --noEmit` rejects calls the runner does not provide, such as `fetch` or a wrong argument type to a helper.

## Versions

The binary, the embedded runner, the runner image tag and the npm package share one version. Pin the binary per project with mise and pin the package to the same version. `cockpit --version` prints it, and `cockpit runner-image` prints the image tag for the runner inside that binary. A new cockpit version keeps `hostApi: 1` as long as it only adds to the host API; a breaking change raises it, and the runner then refuses suites that still declare the old one.
