# Wire record

This page holds the rules of the wire record: which calls a run records, how the record leaves the sealed environment, what is masked and what is never stored, how a call is tied to a step, and what each run state shows. The code is in `internal/wire` (the record), `internal/runner/wire` (the proxy) and `internal/compose/wire.go` (the harness part). When this page and the code disagree, this page is fixed first and the code follows.

The wire record answers one question: what was each server asked, and what did it answer, during a flow or a step? The runner's own records stop at method, host, path and status ([guide](guide.md#correlation-keys-and-request-records)). The wire record adds service-to-service calls, queries, headers and bodies, so it runs only when a project declares it and a submission asks for it.

## Turning it on

Two things are needed. The commit declares what can be recorded, and the submission says that this run records.

```yaml
wire:
  routes:
    - { name: ledger, service: ledger, port: 8081 }
    - { name: orders, service: orders, port: 8080 }
  publicFields:
    - { key: token, value: "tok-[0-9a-f]{8,64}" }
```

```sh
cockpit runs submit --wire --suite smoke
```

`POST /api/runs` takes `"wire": true`, and the form on the Runs screen has a checkbox. A submission with `wire` for a commit without `wire.routes` is refused with the kind `declaration`. A queued run absorbs a new submission only when `wire` matches too. `cockpit run-env` never records.

Service DNS cannot be intercepted, so each address that should pass the recorder reads a variable. The variable holds `host:port` and is set only for a recording run:

```diff
 services:
   orders:
     environment:
-      LEDGER_URL: http://ledger:8081
+      LEDGER_URL: http://${COCKPIT_WIRE_LEDGER:-ledger:8081}/v1
```

Without `--wire` the variable is unset and the service keeps its direct address, under plain `docker compose` too. The name is `COCKPIT_WIRE_` plus the route name in capitals with `-` written as `_`. The hub writes these variables to `env/run.env`, one per route, with the value `cockpit-wire:<41001 + route index>`.

The runner needs no such edit. `env.json` keeps the real addresses and stays the request allowlist. After `ctx.http` has allowed an origin, the runner dials `cockpit-wire` for an origin that matches a route, comparing host names without case, and leaves the URL, the `Host` header and the recorded `host` as they were. Only `http` routes exist.

A value a flow hands to a service, such as a callback URL it registers, is an address the service dials. Write it in `env.json` with the same variable, and the runner expands it before it builds the allowlist:

```json
{ "ordersBase": "http://orders:8080", "ordersCallback": "http://${COCKPIT_WIRE_ORDERS:-orders:8080}/hooks/done" }
```

The runner expands only `${COCKPIT_WIRE_<NAME>:-<default>}`, and only in the string values of `env.json`.

## Declaration rules

| Field | Rule | Rejection kind |
| --- | --- | --- |
| `wire.routes[].name` | matches `^[a-z0-9][a-z0-9-]{0,62}$`, unique | `confinement` |
| `wire.routes[].service` | a service of the compose model, not the builder and not a harness service | `compose` |
| `wire.routes[].port` | `1..65535`; each `service` and `port` pair appears once | `compose` |
| `wire.routes` | at most 64 entries | `compose` |
| `wire.publicFields[].key` | a credential-looking key of the `token`, `session` or `credential` family; a key naming a password, a secret, an API key, authorization or a cookie is refused | `secrets` |
| `wire.publicFields[].value` | RE2 that must match the whole value, and must not match any of the built-in probes: the empty string, 16 lowercase letters, 16 capitals, 16 digits, 32 hex characters, 43 base64url characters, a UUID, a JWT and a mixed password | `secrets` |

The schema carries the name pattern too, as it does for every name field, so a malformed name in a file is refused there first with the kind `schema`.

## The recorder

With `wire` on, the harness adds `cockpit-wire`, a `cockpit-runner wire` process on the runner image. Like every harness service it is read-only, has no capabilities, publishes no port and binds nothing. It joins the internal networks and no other. Every consumer service except the builder gets `depends_on: {cockpit-wire: {condition: service_healthy}}`. A run without `wire` renders the harness of an earlier version byte for byte, so `HarnessVersion` does not change.

`cockpit-wire` listens on one port per route and forwards to `<service>:<port>`. It keeps the caller's bytes and changes only what a direct call would not carry: `Host` becomes the upstream's `host:port`. It uses one new upstream connection per call, because a reused connection the service just closed would surface as a new error. It turns off compression and environment proxies. Bodies stream as they arrive. When the upstream cannot be dialed, the caller's connection is closed, so a retry loop sees a connection error and no invented 502.

The caller's name comes from the Docker DNS PTR record of its address, read as `<project>-<service>-<n>` or `<project>-<service>-run-<id>`. A failed lookup leaves the address.

A person reaches a service through the edge, and the edge may not target a harness service, so edge traffic is not recorded.

### Takes

A take is the part of the traffic that one runner invocation belongs to. The recorder starts recording the take `suite` when it starts. `cockpit-runner wire-ctl`, run by the hub through `docker exec`, drives it over a listener on the container's loopback:

| Command | Effect |
| --- | --- |
| `arm <take>` | clears the record and records under that name (letters, digits, `.`, `_` and `-`, at most 128 characters) |
| `take <take>` | waits until no call is open and none started for 2.5 s, at most 10 s, stops recording and writes the record to stdout, one JSON line per call and one `End` line with the counts and `since`, the time the recorder process started. Calls still open at the limit are written with the outcome `incomplete`. A repeated `take` of the same name returns the same lines until the next `arm`. A `take` of any other name exits non-zero, writes the reason to stderr and no lines |
| `ping` | answers while the proxy listens; the service's healthcheck uses it |

The take of the suite happens in the report stage, where the containers are still up. That holds for a passed, failed, timed-out or aborted run, so a killed runner does not lose the record. Calls that arrive after the take pass through unrecorded.

`cockpit runs exec` arms a take named after the exec result before it starts the runner and takes it after the runner exits, also when the exec is aborted. A recorder that cannot be armed or read costs the exec nothing: the exec answers as usual, with one `wire record:` line in `lines`.

A recorder that restarted lost what it held, and it starts over under the take `suite`. The hub compares `since` with the runner's steps: a `suite` take from a recorder that started after the first step started is `lost`. A recorder restarted during an exec no longer holds the exec's take and refuses it, which is `lost` too.

The record stays in the memory of the process until it is taken. Nothing in the environment reads it over a network, and no recorded byte reaches a disk or a log before the hub has masked it.

| Limit | Value | Past it |
| --- | --- | --- |
| One body, each direction | 1 MiB | omitted whole, `too-large` |
| Bodies waiting to be cleaned | 64 MiB | that call's bodies omitted, `busy` |
| Stored lines with bodies | 96 MiB | later calls keep no bodies, `record-full` |
| Stored lines | 128 MiB | counted in `callsDropped` |

## What a call holds

A call has `seq`, `caller`, `target`, `port`, `method`, `path`, `query`, `started`, `replied` (when the recorder began its last write of the answer to the caller, absent when it wrote none), `finished` (absent while the call is open), `status`, `outcome`, `span` (the span id of a valid `traceparent`), `withheld`, and a `request` and a `response`. Each of those two holds `headers`, `size` and a `body` of one kind:

| Kind | Stored as |
| --- | --- |
| `json` | the parsed value of a JSON media type, or of any body that is a JSON object or array; numbers keep their text, object keys read in sorted order |
| `form` | the parsed fields of `application/x-www-form-urlencoded`; a bracketed name such as `user[password]` reads `user.password`, so the key-name rules reach its last part |
| `text` | valid UTF-8 without a NUL byte |
| `jwt` | the decoded header and claims of a body that is one compact JWT; the signature is dropped |
| `omitted` | nothing but the reason: `too-large`, `binary`, `encoded`, `busy` or `record-full` |

| Outcome | Meaning |
| --- | --- |
| `answered` | the response ran to its end |
| `no-upstream` | the upstream could not be dialed, and the caller's connection was closed |
| `caller-gone` | the caller left before the response ended |
| `incomplete` | the call was still open at the take, or the upstream broke off |

A `gzip` body is decoded within the 1 MiB limit. Any other content encoding is `encoded`. A `multipart` body is `binary`, because its fields have no structure the masker can read. The query is parsed like a form body, and one that does not parse is `text`. Header names are stored in lower case.

## Masking

A body is kept whole or not at all. Nothing is cut, so no fragment of a secret can outlive a match that needed the whole value. A body whose stream broke off, or that the upstream stopped reading, is not stored either; `size` says how much of it passed. The screen shortens only text that is already masked.

One function, `wire.Clean`, runs at every layer with a stronger masker each time. JSON, form and JWT values pass `Masker.Leaves`, so a credential-looking key hides the whole value under it, numbers included. A JSON number that the masker would change reads `****`. Other text passes `Masker.Mask`.

| Layer | Runs in | Masker |
| --- | --- | --- |
| Capture | `cockpit-wire`, in memory | built-in rules and `wire.publicFields` |
| Take | the hub, line by line | the record masker |
| Read | the hub, per request | the run masker with the stored public fields |

The record masker is the run's value sources plus the values the runner captured. While `wire` is on, the runner keeps `work/e2e/wire.private.env.json` (`{"values": [...]}`, 600, replaced atomically) current with every value it masks, minus values a flow marked public with `ctx.ref`. The file stays current up to the moment the runner is killed, and it never enters the report bundle. A later runner of the same run, an exec, starts from that file and adds to it, so a value the suite captured stays masked in the take of an exec even when that exec no longer knows it, or marks it public. The file exists from the moment the runner has started; when a later write fails the runner removes it, so the record of that run is withheld instead of being masked with stale values. These values do not join the run masker: service logs and signals were masked without them, and the bundle's rescan would then find them there. A value a flow captures therefore reads `****` in the record unless the flow marks it with `ctx.ref`.

Header values are hidden by default. Only `accept`, `accept-encoding`, `accept-language`, `cache-control`, `content-encoding`, `content-length`, `content-type`, `date`, `etag`, `host`, `location`, `retry-after`, `server`, `traceparent`, `tracestate`, `user-agent`, `vary`, `x-correlation-id` and `x-request-id` keep theirs, and those pass the masker. Every other header keeps its name and reads `****`.

A JWT is decoded only when it is the whole body. Its header and claims pass `Leaves` like a JSON body, so the value list and the key-name rules reach what base64url hid. A JWT anywhere else reads `****`.

### Public values under credential-looking keys

A field such as `"token": "tok-1f2e"` can be public data under a key the masker hides. `wire.publicFields` keeps a string visible when the key directly above it, or above the array holding it, equals `key` without regard to case and the whole string matches `value`. The value still passes `Mask`, so a known secret or a credential shape stays masked. The exemption applies to structured values in the wire record only: text bodies and every other output of the run keep the normal rules.

The declaration's fields are stored with the record, so a read after destroy applies the same exemption.

### Proof of each call

The hub stores a call only after proving it. `Clean` must not change its own output, and the encoded line must pass the value scan: the run's masker without the key-name rules, so the PEM, known-value, Bearer, JWT and declared-pattern rules. A call that fails becomes a stub with its number, times, method, status, outcome, caller, target and port and `withheld: true`. Its path, query, span, headers and bodies are gone. If the stub's own line fails the scan, its method, caller and target read `****`.

## Where the record lives

A take writes `calls.jsonl` and `index.json` to `<run>/wire/<take>/` through a temporary directory and a rename, with directories 700 and files 600. Once the report bundle exists the takes move to `report/wire/`, and later takes go there. `wire.Store` owns this: `Root` names the directory that holds the record now, `Detach` moves the record out of the bundle and `Attach` moves it in. While `<run>/wire/` exists it is the root, so a take made during a report never lands in the bundle that report is about to replace. The root decides where a take is written. A read looks for a take in both places, so a screen open while the record moves keeps answering.

The report stage runs `Detach`, the `suite` take, `report.Assemble` and, when the bundle was written, `Attach`. The record joins the bundle after `mask.Rescan` has passed, so the rescan never reads it. That is deliberate. The rescan treats a file as one string and would discard the bundle for one call: the PEM rule matches a `BEGIN` in one line and an `END` in a later one, and a public field is a value the key-name rules would mask. Each call carries its own proof instead. If the bundle is discarded for any other reason, the record stays in `<run>/wire/` and still opens. A second report of the same run moves the record back first, finds the `suite` take stored and does not ask the recorder again. The take runs under the report's context with a limit of 3 minutes of its own.

Destroying a run keeps both places: `report/wire/` stays with the bundle. A record still in `<run>/wire/` moves into `report/wire/` when a bundle exists and otherwise stays, as long as the run's record does.

`summary.json` and the bundle's `meta.json` carry `wire`, so a run recorded through the proxy is recognizable in the report. `summary.wire` holds the state of the `suite` take and its counts.

| State | Meaning |
| --- | --- |
| `taken` | the record is stored; `calls`, `withheldCalls`, `bodiesOmitted` and `callsDropped` say what it holds |
| `lost` | the recorder was gone or had restarted, or the environment was down |
| `withheld` | the run's value sources could not be read, so nothing was stored |
| `failed` | the stream broke twice, ended without its `End` line, or the take could not be written |

`withheld` also covers a run whose runner started and left no `work/e2e/wire.private.env.json`. A runner that never started leaves no manifest either, and then the run's own sources are enough. Only `taken` stores anything: the other states leave no take directory, and a later report tries again. A stream is read a second time from its start when it breaks. A recorder that refuses the take writes no line, is `lost` at once and is not asked again.

A state other than `taken` adds one line to `notProven.unread`. The wire record never changes the verdict and never fails the report.

## Tying a call to a step

Linking runs once, at the take, over the calls in start order with `result.json` of the take's runner (the run's or the exec's). All call times come from the recorder's clock.

| Link | Rule |
| --- | --- |
| `exact` | the call's `span` is the span id of a call the runner recorded for a step; or the caller is `cockpit-runner` and the call started inside a step's window, which also covers calls past the runner's limit of 50 per step |
| `nested` | an `exact` or `nested` call to this caller was open for the whole of this call; the tightest such call is the parent and gives the step. No header is needed, so this follows paths that drop `traceparent`. This call counts as over at `replied` when it has one: the caller can act on the answer as soon as the last bytes leave the recorder, and the recorder may not have closed the call yet |
| `time` | the call started inside a step's window and nothing else ties it to the step |
| `none` | the call started outside every step |

A `time` link is never a parent, so a guess cannot spread. A step the runner did not finish stays open until the take, and a call still open at the take encloses every later call to its target.

## Reading the record

`GET /api/runs/{id}/wire` answers a `WireView`, and `GET /api/runs/{id}/wire/{seq}` a `WireCallView` (`docs/api.md`). The Wire screen is `/runs/{id}/wire` (`docs/screen.md`). The index holds one row per call with the offset of its line, the steps of the take's runner with their status, the declared routes and the public fields, so a list reads no body, a picked call reads one line and a destroyed run needs no other file.

| Run | Wire screen |
| --- | --- |
| Without `wire` | no tab; the path answers 404 |
| Queued to testing | says that the record is fixed when the run reaches its report |
| Reported, kept, stopped or destroyed | the stored takes; destroy keeps `report/` and with it the record |
| Resumed | the stored takes; the new recorder process records a `suite` take nobody reads, within its limits |
| After an exec | one more take, chosen on the screen |
| Report bundle discarded | the stored takes, with a notice |

`routes` in the view counts the calls of each declared route, so a route with no calls points at a missing address edit.
