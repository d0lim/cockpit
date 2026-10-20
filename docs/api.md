# Hub API and CLI

This page holds the hub's HTTP routes, the event stream, the request guard and the `cockpit runs` commands with their exit codes. The code is in `internal/hub/api`, `internal/hub/sse`, `internal/hub/cli`, `internal/hub/server` and `internal/hub/labels`. When this page and the code disagree, this page is fixed first and the code follows. `TestAPIDocumentRouteTableMatchesTheRegisteredRoutesBothWays` compares the route table below with the routes the handler registers.

## Starting the hub

`cockpit hub` starts the hub. It:

1. resolves the state directory (`COCKPIT_HOME`, then `$XDG_STATE_HOME/cockpit`, then `~/.local/state/cockpit`),
2. refuses a directory that holds the previous cockpit's layout and exits 3,
3. reads `config.yaml`,
4. takes `hub.lock`,
5. registers its state directory in the hub registry (`docs/screen.md`); a registry it cannot write is logged and does not stop the start,
6. listens on `127.0.0.1` at `COCKPIT_PORT` (default 7710) and on no other address.

`server.Run` takes a `Builder` that returns the scheduler and the optional sources: signals, collector status, run services, suite names and the run masker. The builder receives the shared event bus and the signal stream registry (`server.Streams`, an alias of `signal.Registry`). A component that appends signals must take its stream from that registry, so reads and writes share one index. The builder in `cmd/cockpit` is the run pipeline (`pipeline.New`): it passes the registry to the pipeline, so the report stage, the collector supervisor, the signal and state ports and the API read one stream per run. It answers with the collector supervisor's status, the run's declared ports, the suite names of a worktree's `HEAD` and the run masker, closes the supervisor on shutdown, and publishes each pipeline on the bus as `job` events (`jobId` `<run>/pipeline`, `title` `pipeline`): `start` when it begins, one `out` per masked line, and `exit` with `code` 0 for a passed run and 1 otherwise. Run ports carry their declared `open` mode.

## Guard

Every request passes `api.Guard`:

- The `Host` header must be `127.0.0.1:<port>` or `localhost:<port>`, on every method. This stops DNS rebinding.
- A request with an `Upgrade` header (the screen's WebSocket, `docs/screen.md`) needs `Origin: http://127.0.0.1:<port>` or `http://localhost:<port>`, on every method.
- `POST` also needs `Origin: http://127.0.0.1:<port>` or `http://localhost:<port>` and `X-Cockpit: 1`. It then passes `http.CrossOriginProtection`, which refuses a browser request marked `Sec-Fetch-Site: cross-site`.
- A refusal is 403 with `{"code": "FORBIDDEN", "message": "forbidden:<reason>", "error": "forbidden:<reason>"}`. The reason is `host`, `origin`, `header` or `cross-origin`.

The CLI sends `Origin` and `X-Cockpit` on every write.

## Routes

| Method | Path | Answer |
|---|---|---|
| GET | `/api/events` | event stream for `?target=hub` (default) or `?target=<run id>` |
| GET | `/api/suites` | `SuitesView` for `?worktree=<path>` |
| GET | `/api/runs` | `HubList` |
| POST | `/api/runs` | 202 `Submitted` |
| GET | `/api/runs/{id}` | `RunDetail` |
| POST | `/api/runs/{id}/{op}` | `OpReply` for `keep`, `unkeep`, `stop`, `resume`, `destroy` (200) and `abort` (202, for the active pipeline or a running exec); `ExecView` for `exec` |
| GET | `/api/runs/{id}/report` | `ReportView` |
| GET | `/api/runs/{id}/kit` | `KitView` |
| GET | `/api/runs/{id}/flows/{flow}` | `FlowView` |
| GET | `/api/runs/{id}/source` | `SourceView` for `?file=<path>` |
| GET | `/api/runs/{id}/events` | event stream of that run |
| GET | `/api/runs/{id}/signals` | `SignalsView` for `?after=<seq>&limit=<n>` |

- There is no `/api/state` and no route for a manually started stack.
- Every path outside `/api/` serves the embedded screen (`internal/hub/ui`, built from `web/`) for `GET` and `HEAD`, behind the same `Host` check. Another method answers 404. An unknown `/api/` path answers a 404 failure body, never the screen.
- Every `{id}` must match the run id pattern (400) and name a known run (404).
- A failure is `{code, message}`, plus `files`, `run` (a `RunItem`) or `candidates` when they apply. The codes are listed in `api.Codes`.
- The submit body is `{worktree, commit, suite?, submitter?, new?}`. The submitter is the body field, then the `X-Cockpit-Submitter` header, then `ui`. `?force=1` on an operation acts on another person's run.

### Response shapes

The tests check every answer against a JSON Schema generated from the Go struct.

Because that schema follows the struct, it cannot see a field that disappears. `internal/hub/api/testdata/response-fields.json` is the committed baseline of every field path of `RunItem`, `RunDetail`, `HubList`, `ReportView`, `ExecView`, `FlowView`, `SourceView`, `SignalsView`, `KitView` and `Problem`, with its wire type (`string`, `integer|null`, `array of object`, `map of string`, `custom <Go type>` for types with their own JSON encoding) and `, optional` when the field is omitted when empty. `TestEveryResponseFieldInTheBaselineKeepsItsNameAndTypeAndNewFieldsPass` fails when a baseline field disappears, changes type, or becomes optional, and passes when fields are added. After adding a field, record it so it is protected too: `COCKPIT_UPDATE_API_BASELINE=1 go test ./internal/hub/api -run TestEveryResponseFieldInTheBaselineKeepsItsNameAndTypeAndNewFieldsPass`, and commit the diff. A removal or a type change is a breaking change of the API: regenerate the baseline the same way only when the break is deliberate, and say so in the commit message.

- **`RunItem`**: the run's state and identity fields, `phaseLabel` and `verdictLabel`. It keeps every field the previous hub sent except its harness flag, and adds `hostApi` and `cockpitVersion`.
- **`RunDetail`**: a `RunItem` plus image keys and digests, the runner image, the tail, `allowedEvents` (`stop`, `resume`, `destroy`, `abort`; only `abort` while an exec holds the run) and `services`. Each service is `{name, service, port, hostPort, label, open?}`, where `open` is the declaration's open mode (`browser`, `link` or `copy`).
- **`HubList`**: `runs` (`RunItem` rows), `orphans`, `foreign`, `hubs`, `pool`, `up`, `dockerDown`, `tickError` and `servedHostApis`, the host API versions this hub serves. An orphan is `{project, running, hub?}`, where `hub` is the id of the registered hub that holds the run's record. `foreign` names the running compose projects without the cockpit label. `hubs` is the hub registry, this hub first: `{id, name, home, self, up, url?}`, with `url` only while the hub is up. The screen hides the signal and report tabs of a run outside that set.
- **`ReportView`**: one of four statuses.
  - `pending` while the run is queued or active.
  - `redaction-failed` with the bundle paths the rescan caught.
  - `meta-only` with the failed stage, notice and tail when there is no bundle.
  - `bundle` with `summary`, `meta`, `flows`, `steps`, `stoppedAt`, `notProven` and `reportDir`.
- **`KitView`**: services, kit files, the `cockpit runs exec` command with the repeatable flows, and the report directory.
- **`ExecView`**: `exitCode`, `oomKilled`, `timedOut`, `flows`, `lines` and `reason`.
- **`FlowView`** and **`SourceView`**: read from the bundle when it exists (`source: bundle`). Otherwise they are read live from `work/e2e` and `work/scripts` and pass the run masker (`source: live`). The live source view serves only files the manifest points to, and it refuses to answer without a run masker.
- **`SignalsView`**: the signal read (`signals`, `frozen`, `frozenSeq`, `dropped`, `resync`, `next`) plus `collectors`. Each collector entry is `{id, state, lastOkAt, failures, reason}`, where `state` is a `collect.State`: `armed`, `backoff`, `stopped` or `report-only` (a collector with no interval that runs only at the report). A run whose reads are refused answers 409 `READ_REFUSED`.

## Event stream

The screen keeps one connection per stream target for the whole browser, because a browser allows only six HTTP/1.1 connections to one origin and every open stream holds one. `web/live` does this in three ways, tried in order:

1. A `SharedWorker` (`/live/worker.js`, name `cockpit-live-<protocol>`) owns one `EventSource` per target that any tab asks for. It sends each new tab the frames it has kept for that target, fans live frames out to every tab that subscribed, and closes a target's stream when the last tab leaves. A tab says `bye` on `pagehide` and pings every 10 seconds; a tab silent for 35 seconds is dropped.
2. Without a working shared worker, the tabs elect a leader through the Web Locks API and talk over a `BroadcastChannel`. The leader holds the streams and broadcasts frames. When it closes, the next tab takes the lock and opens the streams again.
3. Without either, each tab opens its own streams, one per target.

Every layer drops a frame whose composite id is not newer than the last one it delivered for that target (bus seq for `runs` and `job`, signal seq for `signal` and `resync`, one `signal-refused` per epoch), so a reconnect or a leader change repeats nothing. A stream's frames are:

- `runs`: the whole `HubList`, sent whenever it changes.
- `job`: one `sse.JobEvent` from the run's target, `{jobId, title, phase, text?, code?}`. `phase` is `start` (`text` is the command, when there is one), `out` (`text` is one output line) or `exit` (`code` is the exit code, present even when it is 0). The pipeline publishes it with `Bus.PublishJob` and the constructors `JobStarted`, `JobOutput` and `JobExited`.
- `log` and `services`: from the run's target.
- `signal`: one `contract.Signal` per frame, and it passes `schema/signal.json`.
- `resync`: a `{from, to}` gap of dropped signals. The screen keeps the gap notice and reads `GET /api/runs/{id}/signals?after=<to>` page by page, following `next`, up to its 20,000-signal cap. It merges the retained signals by `seq` with the ones the stream delivered meanwhile and adds any further gap the read reports. A failed read leaves the stream signals as they were.
- `signal-refused`: the run's signal reads were refused.

Rules:

- Each frame carries `id: <epoch>.<bus seq>.<signal seq>`. The epoch is the hub start time in milliseconds. The bus seq counts in memory and restarts at 1 when the hub restarts.
- A reconnect sends `Last-Event-ID`. With the same epoch, the stream resends only bus events after the bus seq and signals after the signal seq. With another epoch, or with a malformed id, it starts from the beginning.
- On subscribe, the stream replays the buffered events of the target and of the hub in seq order. The buffer holds 1000 events per target and kind. `runs` and `services` keep only their last snapshot.
- A subscriber that falls 256 events behind is dropped, and its client reconnects with its last id.
- The stream sends `: ping` every 15 seconds and flushes with `http.ResponseController`.

## CLI

`cockpit runs <submit|list|get|show|report|wait|keep|unkeep|stop|resume|destroy|abort|exec>` calls the hub at `COCKPIT_URL` (default `http://127.0.0.1:7710`).

- `submit` sends the git top level of the current directory, `--commit` (default `HEAD`), `--suite`, `--submitter` (then `COCKPIT_SUBMITTER`, then `cli`) and `--new`.
- `--wait` and `wait` poll the run until it settles. They reconnect for 60 seconds when the hub goes away.
- `exec <id> -- <flow id>` reruns one flow in a finished run whose environment is up.
- `--json` prints exactly one JSON object line: `{"schemaVersion": 1, "code": <stable code>, ...}`. The stable codes are `cli.StableCodes()`: the CLI's own codes plus the hub's `api.Codes`.

| Exit | Meaning |
|---|---|
| 0 | success, or verdict `passed` |
| 1 | verdict `failed` (also a runner exit code 1 from `exec`) |
| 2 | verdict `error`, a hub error (5xx) or an unexpected CLI error |
| 3 | refused (4xx other than 409) or a usage error |
| 4 | the hub does not respond; the message names `cockpit hub` |
| 5 | conflict (409) or verdict `aborted` |
| 6 | `--timeout` passed; the run keeps going |
| 7 | waiting on a person: the queue is blocked because every kept environment has keep on (`pool-all-kept`) |

## Labels

`internal/hub/labels` owns the words the screen and the CLI show: state, stage, wait, stop and destroy labels from `internal/hub/lifecycle`, verdict, collector state and flow status labels, and the CLI messages with `{name}` placeholders. `go generate ./internal/hub/labels` writes `web/shared/labels.json`. `make verify` fails when the file drifts, and a test compares it with the Go table.

## Screen types

`go generate ./internal/hub/api` writes `web/shared/api.ts` from the Go response structs with `internal/hub/api/tsgen`. A field with `omitempty` or `omitzero` is optional, a pointer without it is `T | null`, an embedded struct becomes `extends`, and a named string type becomes the union of its typed constants. `lifecycle.Verdict` marshals its zero value as `null`, so it renders without the empty value and every field of that type is `Verdict | null`. Any other type with its own `MarshalJSON` stops the generator until it gets a TypeScript form. A test compares every generated interface with the JSON Schema that `jsonschema.ForType` builds from the same struct, property by property and required field by required field. The screen imports these types through `web/shared/types.ts` and adds only its own: operation names, frame envelopes and the data source badge.
