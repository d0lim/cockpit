# Queries for a bundle archive

Run every command inside the directory the archive extracts to. `<...>` marks a value to fill in.

The time in a run id is the local time of the machine that made the run. An id that ends in `-h<n>` comes from a run made without a hub, for example in CI. Every timestamp inside the archive is UTC.

## Verdict

```sh
jq '{runId, commit, suite, verdict, reasons, steps, stoppedAt, notProven: (.notProven | map_values(length)), seal, boot, wire, cockpitVersion}' report/summary.json
jq '.runner // {} | del(.lines)' report/summary.json
jq '.stages | to_entries[] | select(.value.status != "ok")' report/summary.json
cat report/not-proven.md
```

- `verdict` is `passed`, `failed`, `error` or `aborted`. `failed` says a step failed or a seal was broken. `error` says the run could not be judged as a test: the environment did not come up, the runner died, or its result is missing or inconsistent. Read `reasons` before calling an `error` a product defect.
- `reasons[0]` names the rule that held. When the run has a stop point, `reasons[1]` is `flow › step — message`, the runner's own text.
- `steps` counts `expected` (the manifest), `actual` (the result), `failed`, `skipped` and `requests`.
- `notProven` is counted by the first query. `report/not-proven.md` holds the entries as text, ready to quote.
- `seal` is the seal check: `passed`, `failed` or `error`. `network` is `sealed` or `open`; at `open` the services could reach outside the run, the probes did not run, and `notProven.declared` says so, so an outside dependency is a possible cause of a failure. `report/seal.json` lists every assertion under `static` and every probe under `probes`, each with `name`, `ok`, `expected` and `actual`.
- `boot` says whether the expected services came up, and which were still `waiting`.
- `runner` holds the runner's exit `code`, `oomKilled`, `timedOut`, `aborted` and its last output `lines`.
- `stages` lists the hub's own readers (`runner.<file>`, `source`, `signals`) and one entry per collector. `absent` means there was nothing to read, which is the normal state of `runner.exec` in a run where no flow was run again. `error` is the status to report.
- `report/meta.json` is the run record: `submitter`, `createdAt`, `startedAt`, `reportedAt`, `declaration` when the run read a declaration other than `.cockpit/project.yaml`, and for a run that stopped before its suite `failedStage`, `notice` and `tail`, the last lines of the stage that failed.

## Flows and steps

```sh
jq -r '.flows[] | [.status, .id, .stage, "\(.passed)/\(.steps)", .stoppedAt // ""] | @tsv' report/summary.json
jq '.flows[] | select(.status != "passed") | {id, status, requires, stoppedAt}' report/summary.json
jq --arg f <flow> '.flows[] | select(.id == $f) | .stepList[] | {id, title, kind, status, message, source, requests, httpStatus, durationMs}' report/summary.json
jq --arg f <flow> --arg s <step> '.flows[] | select(.id == $f) | .stepList[] | select(.id == $s) | {status, message, source, calls, refs}' report/summary.json
```

- A flow is `passed`, `failed`, `skipped`, `notRun` or `incomplete`. A step is `passed`, `failed`, `skipped`, `running` or `notRun`.
- When a step fails its flow stops there. A flow that `requires` a failed flow is `skipped`. To explain a skipped flow, follow `requires` back to the flow that failed.
- `once: true` marks a flow that runs once per environment and whose state later flows reuse.
- A step fails when one of its expectations does, and `message` says which. A call that answered 400 or above is not a failure by itself: a step may expect it. `httpStatus` is the status of the step's last response.
- A step's `calls` are the requests it sent (`method`, `host`, `path`, `status`, `startedAt`, `durationMs`, `spanId`), at most 50 per step. They carry no `seq`. Its `refs` are the ids it marked, which the signals carry as `ref.<name>`.
- `report/e2e/result.json` holds the same steps as the runner wrote them, with `startedAt` and `finishedAt`. `report/e2e/manifest.json` holds the suite as declared: stages, flows, steps and their source positions.
- `report/e2e/exec/<name>.json` is a flow that was run again after the run ended. It is not part of the verdict.
- `report/e2e/ledger.json` holds the last status of every flow in this environment, later execs included. `report/e2e/context.json` holds the values the flows captured for later flows; a credential among them reads masked.

## Source

```sh
jq '.files | keys' report/source/source-index.json
sed -n '<from>,<to>p' report/source/<file>
```

The bundle holds the files that declare the suite's flows and steps, the ones the manifest points to. A module those files import is not in the archive, and no list names it: write that the archive does not hold it. `omitted` in `source-index.json` names a declaring file the bundle could not hold, with the reason.

## Service calls

Only a run submitted with `wire` has `report/wire/`. For any other run `jq '.wire' report/summary.json` is `null` and the queries below have no file to read. `suite` is the take of the run itself; any other take belongs to a later exec.

```sh
jq '.status, .routes' report/wire/suite/index.json
jq -r '[.heads[].link.kind] | group_by(.) | map("\(.[0]) \(length)") | .[]' report/wire/suite/index.json
```

The calls of one step, in the order they started:

```sh
jq -r --arg f <flow> --arg s <step> '
  (.steps | to_entries[] | select(.value.flow == $f and .value.step == $s) | .key) as $n
  | .heads[] | select(.link.step == $n)
  | [.seq, .link.kind, .caller, .target, .method, .path // "", .status, .durationMs] | @tsv
' report/wire/suite/index.json
```

One call with its headers and bodies:

```sh
jq --argjson q <seq> 'select(.call.seq == $q) | .call' report/wire/suite/calls.jsonl
```

- `link.step` is an index into `.steps` of the same index, and `-1` when the call belongs to no step. `link.parent` is the `seq` of the call that caused this one.
- `exact`: the runner sent the call in that step. `traced`: its span chain reaches such a call. `nested`: it ran entirely inside such a call to its caller. `time`: it only started while the step ran. `background`: its chain starts at a scheduled task or a consumer, named in `link.root`. `none`: it started outside every step.
- A body has one `kind`: `json`, `form`, `text`, `jwt` or `omitted` with the reason. A body is kept whole or not at all.
- Most header values read `****` by design; only a short list of plain headers keeps its values.
- `outcome` is `answered`, `no-upstream`, `caller-gone` or `incomplete`.
- `status.withheldCalls`, `bodiesOmitted` and `callsDropped` say how much of the record is missing.
- The `seq` of a wire call counts the calls of its take. It is not the `seq` of a signal.

## Signals

One JSON object per line: `seq`, `ts`, `tsSource`, `kind` (`span`, `event`, `metric`), `collector`, `attrs`, a body under `span`, `event` or `metric`, and `id` on a row that can be revised.

```sh
jq -r '[.collector, .kind] | @tsv' report/signals/stream.jsonl | sort | uniq -c
jq -r 'select(.collector == "logs") | .attrs.container' report/signals/stream.jsonl | sort | uniq -c
```

The hub writes these collectors itself, each only when it had something to record. Every other id is a collector of the project:

| Collector | Holds |
| --- | --- |
| `pipeline` | the root span `pipeline` and one span per stage that had ended: `prepare`, `build`, `seal`, `boot`, `suite` |
| `runner` | the spans `run`, `flow:<flow>`, `step:<flow>/<step>` and `call:<flow>/<step>/<n>` |
| `logs` | container log lines, with the container name in `attrs.container` |
| `services` | the state of each service |
| `supervisor` | the status of the collectors |
| `otlp` | spans and metrics the services exported |

How long each stage took, and each step:

```sh
jq -r 'select(.collector == "pipeline" and .kind == "span") | [.span.name, .span.start, .span.end // "", .span.status] | @tsv' report/signals/stream.jsonl
jq -r 'select(.collector == "runner" and (.id | startswith("step:"))) | [.id, .span.start, .span.end // "", .attrs.status] | @tsv' report/signals/stream.jsonl
```

The requests of one step as signals, each with its `seq`:

```sh
jq -r --arg p 'call:<flow>/<step>/' 'select(.collector == "runner" and ((.id // "") | startswith($p))) | [.seq, .span.name, .attrs.status // "", .span.start] | @tsv' report/signals/stream.jsonl
```

Everything recorded while one step ran: take its window, then read the window.

```sh
jq -r --arg id 'step:<flow>/<step>' 'select(.collector == "runner" and .id == $id) | [.span.start, .span.end // ""] | @tsv' report/signals/stream.jsonl
jq -c --arg a <from> --arg b <to> 'select(.ts >= $a and .ts <= $b) | {seq, ts, collector, kind, id, attrs, body: (.event.body // .span.name // .metric.name)}' report/signals/stream.jsonl
```

Write `<from>` and `<to>` without the trailing `Z`, and take `<to>` a little past the end. Times are compared as text and carry between three and nine fractional digits, so a bound that ends in `Z` drops rows inside its last millisecond.

Everything recorded about an id that a step marked:

```sh
jq -c --arg v <value> 'select([.attrs | to_entries[] | select((.key | startswith("ref.")) and .value == $v)] | length > 0)' report/signals/stream.jsonl
```

The log lines of one container between two times, bounds written the same way:

```sh
jq -r --arg c <container> --arg a <from> --arg b <to> 'select(.collector == "logs" and .attrs.container == $c and .ts >= $a and .ts <= $b) | [.seq, .ts, .event.body // ""] | @tsv' report/signals/stream.jsonl
```

- `tsSource` says whose clock wrote `ts`: `host`, `container` or `runner`. Compare times across sources with that in mind.
- A stage span's `status` says whether the stage ran to its end, not what the suite found: a run with failed steps still has an `ok` `suite` stage. A call span is `error` when its response was 400 or above. A span that is `incomplete` was still open when the runner stopped.
- A row with an `id` appears once per collector and kind: the archive holds the last revision of each, not its history.
- A log row with an empty `event` is an empty line.
- Each collector and each container has a record limit. What was dropped is counted in `notProven.unread`.

## Two runs

Extract each archive into its own directory, then compare what should match and what should not:

```sh
jq -r '[.commit, .suite, .verdict, .cockpitVersion] | @tsv' <a>/report/summary.json <b>/report/summary.json
jq -r '.flows[] | .id as $f | .stepList[] | [$f, .id, .status, .durationMs // ""] | @tsv' <a>/report/summary.json | sort > a.tsv
jq -r '.flows[] | .id as $f | .stepList[] | [$f, .id, .status, .durationMs // ""] | @tsv' <b>/report/summary.json | sort > b.tsv
diff a.tsv b.tsv
```

`images.keys` in each summary says which images were built from different inputs. One run's timing is one sample: call a step slower only when the difference is large against the step's own duration, and say that it is one run against one.

## Writing it up

State, in this order:

1. The run id, commit and suite.
2. The verdict with its reasons, as the hub wrote them.
3. For a run that did not pass: the flow and step where it stopped, the source position, the message, and the calls or signals that show the cause. Give each signal or wire call its `seq` and say which of the two it is. A step call has none: give its method, path and status.
4. What was not proven: the count of each list under `notProven`, the entries themselves when they are few, and otherwise the entries grouped by flow or stage with a pointer to `report/not-proven.md`.
5. What the archive does not hold that the question needed.
