# OTLP traces and metrics plan

A run's reader today sees the runner's run, flow, step and call spans, but nothing the services under test did inside a step, and no hub stage timing.
This program makes the hub record its own stage spans, makes the runner send a W3C `traceparent` on every call, and adds a sealed OTLP/HTTP receiver whose spans and metrics land in the run's signal stream under the call that caused them.
It assumes the consumer services export OTLP over HTTP once the hub sets the standard `OTEL_*` variables (Appendix D holds the consumer recipe).
The rule the program enforces is that no telemetry leaves the sealed network and every received value passes the run redactor before it is stored.
The PRs in order are PR1 `stage-spans`, PR2 `traceparent`, PR3 `metric-series`, PR4 `otlp-receiver`, PR5 `otlp-harness`, PR6 `otlp-ingest`.

## How to read this

One box is one unit of work. Every box names the evidence that checks it. A nested box is a sub-step of the box above it. Check a box only when its evidence exists, a file, a log line, a screenshot, a test run, or a SHA. The body is a how-to. The appendices explain and record.

The program runs the pstack `playbooks/autopilot-stack.md` playbook when an agent drives it. A developer driving it by hand follows the same boxes. Nobody in the program merges. The operator lands every PR, so PR1 to PR6 all stop at merge-ready.

Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

Repository rules still bind every PR. Read `AGENTS.md` before the first edit. Documentation changes first and code follows it. No comments. No file over 300 lines. `make verify` is the only gate.

## Program checklist

### Arm the program

- [ ] State the protocol and this plan to the operator, then stop. Start execution only on the operator's explicit go.
- [ ] Read these from trunk at program start. Re-read them at every tick.
  - [ ] `git show origin/main:AGENTS.md`
  - [ ] `git show origin/main:docs/signals.md`
  - [ ] `git show origin/main:docs/sealing.md`
  - [ ] `git show origin/main:docs/harness.md`
  - [ ] `git show origin/main:docs/screen.md`
  - [ ] The pstack `playbooks/autopilot-stack.md`, `skills/swarm/SKILL.md` and `playbooks/opening-a-pr.md`, read from the installed pstack plugin because this repository does not vendor pstack.
- [ ] On the operator's go, arm the audit tick as `/loop 1h` with the tick prompt below. Never leave the cadence to memory.
- [ ] Use this tick prompt, verbatim. "Re-read the execution playbook from trunk. Audit the operation against it and fix drift in this tick. Probe every active lane and judge progress by side effects only. Stand down a stuck lane and dispatch its replacement now. Then post a short status message to the operator in chat only when the audit found a tracked change that no earlier status message reported, such as a PR opened, a code-ready head, a round launched or closed, a verdict, a merge, a stuck agent and the action taken, a blocker added or cleared, or a decision only the operator can make. Name every such change and nothing else. Do not repeat a table, the merged list, or an unchanged blocker. If the audit found none, end the turn with no reply text. Either way, log this tick's row in your decision trail. The row names the items reported, or none."
- [ ] On the operator's hold or stand-down, send every owner a zero-writes order at once.

### Spawn owners

- [ ] Spawn one owner per PR with the full lifecycle the execution playbook names.
- [ ] Follow this dependency graph. Base a dependent PR on its parent branch.
  - [ ] PR1, PR2 and PR3 are independent and first. All three branch from `main`.
  - [ ] PR4 after PR3.
  - [ ] PR5 after PR4.
  - [ ] PR6 after PR2 and PR5. Rebase PR6 onto PR2 once PR2 lands, or base it on a branch that merges both.
- [ ] Hold the file boundaries.
  - [ ] PR1 touches `internal/hub/pipeline`, `internal/hub/store`, `internal/hub/screen`, `internal/signal/spans.go`, `internal/compose/collectors.go` and their docs.
  - [ ] PR2 touches `internal/tracectx`, `internal/ports/http`, `internal/runner`, `internal/contract/result.go`, `internal/signal/calls.go`, the runner environment in `internal/compose/harness.go` and the seal fixture.
  - [ ] PR3 touches `internal/contract/signal.go`, `internal/signal`, `internal/hub/screen/signal*.go`, `sdk/`, `schema/` and the host API fixture.
  - [ ] PR4 touches `cmd/cockpit-runner`, `internal/runner/otlp`, `go.mod` and `go.sum`.
  - [ ] PR5 touches `internal/contract/declaration*.go`, `internal/compose`, `internal/seal`, `internal/ports/docker`, `internal/hub/fakedocker`, `testdata/toy-project` and the seal fixture.
  - [ ] PR6 touches `internal/collect/observer`, `internal/hub/envops`, `internal/hub/pipeline`, `internal/signal/caps.go`, `internal/hub/screen` and their docs.
- [ ] Hold the review gate. PR1 and PR6 change what the screen shows. They wait for the operator's review in chat with screenshots and a video before merge.

### PR mechanics, for every PR

- [ ] Resolve the forge once. Default to `gh`; if `command -v origin` succeeds and Origin can resolve the repository, use `origin pr` for every PR operation. Record any fallback to `gh`. Never require `gt`.
- [ ] Open the PR ready, never draft, per **Opening a PR**. Use `gh pr create --base <base-branch>`. A stack child targets its parent branch.
- [ ] Run `make verify` once before the PR-facing push. Push with hooks on.
- [ ] Run `/deslop` before each commit and `/no-comments` before review. The repository's no-comment check enforces the same rule.
- [ ] Triage every review comment on its merits. Dismiss noise with a concrete reason.
- [ ] Rebase onto current trunk before the code-ready report. Keep that merge base in fix rounds. Rebase again only at merge prep, on a `git merge-tree` conflict with trunk, on a CI failure that comes from a change on trunk, or when a known concurrent change to the same files lands.
- [ ] Commit messages follow `type(scope): English summary`.

### Verdict and merge, for every PR

- [ ] A full swarm runs once, at the code-ready head. Another full swarm runs only for new behavior, a redesign, or a large new diff. A rebase, a merge-conflict resolution, or a minor fix gets a single verifier at the new head, and a wording-only change gets none. Each full swarm has one gates lane that runs `make verify` and the integration tests, the ten live lanes from the PR's **Verify, live** block, the perf lane from its **Verify, perf** block, and two or more audit lanes that read the diff and the receipts and distrust the PR body. The root audits the receipts in the merge-ready report before the verdict.
- [ ] Judge every finding by the execution playbook's blocking rule. The verdict is clean when none blocks. A failure on trunk behavior in a lane marked `Regression lane against trunk.` is recorded against trunk and does not block the PR. A new head gets a fresh verdict, except for results that stay valid under the patch-id rule in the pstack `playbooks/shipping.md`.
- [ ] Stop each PR at merge-ready with its verdict, its receipts and its head SHA. The operator lands the stack bottom-up. A rebase after the verdict keeps the verdict only when the patch-id is unchanged.

### Boot recipe, for every live lane

Each live lane runs on a Mac with Docker Desktop at the PR head, because the seal probes reach the host canary through `host.docker.internal` and Linux CI skips those tests. Drive the CLI directly and the screen through a browser.

- [ ] `git fetch origin <head-branch> && git worktree add ../lane-<n> <head SHA>`.
- [ ] Run `make build`, then `bin/cockpit runner-image`, then start the hub with `bin/cockpit hub` and wait for the "hub listening" log line.
- [ ] Submit the toy consumer with `make run-env` or `bin/cockpit runs submit` against a temporary git copy of `testdata/toy-project`, as `scripts/run-env.sh` does. Read state only through `bin/cockpit runs` subcommands, the hub routes in `docs/api.md`, and the screen.
- [ ] Save every capture to `/tmp/swarm-<pr-id>/worker-<n>/<slug>` and return the paths with the report. A `.png` is a screen capture. A `.txt` or `.json` is CLI or route output.

## Record hub stage spans (PR1)

**Depends on.** None.

**Files.**

- [ ] Edit `docs/signals.md` first. Add a "Stage spans" section and list every reserved collector id, which also closes today's gap where only `runner` and `logs` are named.
- [ ] Edit `docs/screen.md` for the overview sentence and the trace tree root.
- [ ] Edit `internal/hub/store/meta.go`.
- [ ] Edit `internal/hub/pipeline/run.go` and `internal/hub/pipeline/spans.go`.
- [ ] Create `internal/hub/pipeline/stagespans.go` and `internal/hub/pipeline/stagespans_test.go`.
- [ ] Edit `internal/signal/spans.go` so the runner's `run` span takes `stage:suite` as its parent.
- [ ] Edit `internal/compose/collectors.go` to reserve the collector id `pipeline`.
- [ ] Edit `internal/hub/screen/run.go`.

**Build.**

- [ ] Add `Stages []StageTime` to `store.Meta`, where `StageTime` holds the stage label, start, end and outcome. `Hub.Pipeline` writes the start when it picks a stage with `stageFor` and the end when `ApplyEvent` or `failure` returns. The meta persists it, so a hub restart keeps the times.
- [ ] In the report stage, next to `runSpans`, append one root span `pipeline` and one span `stage:<label>` per entry in `Meta.Stages`, all with the run id as the trace id and the collector `pipeline`. Append them before `Freeze`, so the frozen copy holds them. The report stage's own span ends after `Assemble` and goes to the tail.
- [ ] Do not open the stream during prepare. The masker is cached by `Ops.masker` and could go stale before `GenerateEnv`. Building the spans from the meta at report time avoids that path.
- [ ] Set `ParentID` of the runner's `run` span to `stage:suite` in `signal.RunSpans`, so the tree reads pipeline, stage, run, flow, step, call.
- [ ] Change the overview sentence to "Running the X stage since <stage start>".

**You see.**

- [ ] The signals trace view of a finished toy run shows the root `pipeline` with children `stage:prepare`, `stage:build`, `stage:seal`, `stage:boot`, `stage:suite` and `stage:report`, each with a duration, and the existing `run` tree under `stage:suite`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `stagespans_test.go` gains `TestEveryStageTheLoopRanBecomesASpanUnderThePipelineRoot` and `TestAFailedStageEndsWithAnErrorSpanAndLaterStagesHaveNone`. Run `go test ./internal/hub/pipeline`.
- [ ] `internal/hub/store` gains `TestStageTimesSurviveAHubRestart`. Run `go test ./internal/hub/store`.
- [ ] `internal/signal` spans tests gain a case where the `run` span's parent is `stage:suite`. Run `go test ./internal/signal`.
- [ ] `internal/compose/collectors_test.go` gains a deny case for a consumer collector named `pipeline`, with the allow canary of a neighbouring id, and a row in its mutation record. Run `go test ./internal/compose`.
- [ ] Run `make verify`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `inherit-parent` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run one passing toy run at trunk and at head. Trunk has no stage spans, so record that and gate the head trace tree plus the passing verdict. Save `trace-tree.png`. Pass when the head verdict is pass and the tree shows six stage spans under `pipeline`.
- [ ] Lane 2. Break the toy `Dockerfile` so the build fails. Save `build-failed.png`. Pass when `stage:build` has status error and no `stage:seal` or later span exists.
- [ ] Lane 3. Restart the hub during the suite stage and let the run finish. Save `restart.json` from the signals route. Pass when every stage span has a start earlier than the restart and none is duplicated.
- [ ] Lane 4. Destroy a finished run and read its signals. Save `after-destroy.json`. Pass when the response says `source: bundle` and holds the stage spans.
- [ ] Lane 5. Run `bin/cockpit runs exec` on a kept run. Save `exec-trace.png`. Pass when the exec trace is unchanged and separate from the pipeline tree.
- [ ] Lane 6. Watch the overview during the boot stage. Save `overview-boot.png`. Pass when the sentence shows the boot stage start, not the run start.
- [ ] Lane 7. Stop and resume a kept run. Save `resume.json`. Pass when resuming adds no second set of stage spans.
- [ ] Lane 8. Open the report bundle's `signals/stream.jsonl`. Save `bundle-stages.txt`. Pass when the stage spans are in the frozen copy with collector `pipeline`.
- [ ] Lane 9. Submit a consumer whose declaration names a collector `pipeline`. Save `reserved.txt`. Pass when the submission is refused with the reserved id named.
- [ ] Lane 10. Open the trace view of a run with a failing step. Save `failing-step.png`. Pass when the failing step sits under `run`, under `stage:suite`, and keeps its error status.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Wall time from submit to verdict for the toy run, and the report stage duration from the new stage span.
- [ ] Probe. Three toy runs at trunk and three at head, interleaved trunk, head, trunk, head, on one idle machine.
- [ ] Baseline. Record the trunk median wall time first.
- [ ] Rule. Fail when the head median wall time exceeds the trunk median by more than 2 percent, or when appending the stage spans adds more than 50 ms to the report stage.

**Review gate.** The operator reviews before merge.

- [ ] Copy lane 1 and lane 2 screenshots into `docs/plans/media/pr1-review-<slug>.png` on the lane machine, not in the commit.
- [ ] Record a 30 to 60 second video of the trace view on a finished run. Save it as `pr1-review.mp4`.
- [ ] Post the screenshots and the video in chat. Stop at merge-ready. Wait for the operator's click.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Review comments triaged.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends PR1 to the stack and the operator lands it.

## Send a traceparent on every runner call (PR2)

**Depends on.** None.

**Files.**

- [ ] Edit `docs/signals.md` first. Describe the run's W3C trace id, the `spanId` call attribute and the rule that a script's own `traceparent` wins.
- [ ] Edit `docs/guide.md` for flow authors.
- [ ] Create `internal/tracectx/tracectx.go` and `internal/tracectx/tracectx_test.go`.
- [ ] Edit `internal/ports/http/http.go` and `internal/ports/http/http_test.go`.
- [ ] Edit `internal/runner/settings.go` and `internal/runner/records.go`.
- [ ] Edit `internal/contract/result.go` and `internal/contract/docs.go`, then run `go generate ./...` for `schema/`.
- [ ] Edit `internal/signal/calls.go`.
- [ ] Edit `internal/compose/harness.go` and raise `HarnessVersion`.
- [ ] Edit the hub code that starts the runner for a suite and for an exec, where `E2E_EXEC_OUT` is set today.
- [ ] Regenerate `internal/seal/testdata/config.toy.json` with `make seal-fixture`.

**Build.**

- [ ] `tracectx.TraceID(runID)` returns the first 16 bytes of SHA-256 over `cockpit/` plus the run id, as 32 lowercase hex characters. `tracectx.ExecTraceID(runID, name)` does the same over the exec trace name `<run id>/exec/<name>`. Both are pure, so the hub can compute them without asking the runner.
- [ ] The hub passes the trace id to the runner container as `COCKPIT_TRACE_ID`, the same way it passes `E2E_EXEC_OUT`. The runner does not know its run id today, and `loadSettings` gains this one variable. The run id is not a secret, since it is already a compose label.
- [ ] `Client.Send` sets `traceparent` to `00-<trace id>-<span id>-01` after the script's headers, with a fresh random 8-byte span id per call. When the script already set `traceparent`, the script's value goes out unchanged and the call records the span id parsed from it. A value that does not parse goes out unchanged and records no span id.
- [ ] `Call` gains `SpanID`. `Driver.takeRecords` copies it into a new optional field `contract.StepCall.SpanID` (`spanId,omitempty`). The field is additive, so `RunnerSchemaVersion` stays at 2 and the host API does not change, because `StepCall` is not in `sdk/`.
- [ ] `callSpans` adds the attribute `spanId` to each call span. PR6 uses it to parent service spans.
- [ ] With no `COCKPIT_TRACE_ID`, the runner sends no `traceparent`. This keeps an old hub and a new runner from inventing trace ids, although the revision check already ties them together.

**You see.**

- [ ] A toy call span in the signals table carries `spanId` with 16 hex characters, and the toy `app` log line for that request shows the matching `traceparent`.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `tracectx_test.go` gains `TestTheTraceIDIsStableForARunAndDiffersBetweenRuns` and `TestATraceparentRoundTripsThroughFormatAndParse`, with a malformed corpus whose case count is asserted. Run `go test ./internal/tracectx`.
- [ ] `http_test.go` gains `TestEveryCallSendsATraceparentWithTheRunTraceID`, `TestAScriptTraceparentWinsAndItsSpanIDIsRecorded` and `TestNoTraceIDMeansNoTraceparent`, and a row in its mutation record. Run `go test ./internal/ports/http`.
- [ ] `internal/runner` gains `TestTheStepCallCarriesTheSpanIDThatWasSent`. Run `go test ./internal/runner`.
- [ ] `internal/signal` gains `TestACallSpanCarriesTheSpanIDAttribute`, and the hub append path proves a 16-hex span id reads back unmasked next to a planted secret that reads back as `****`. Run `go test ./internal/signal`.
- [ ] Run `make generate-check` and `make verify`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `inherit-parent` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run one passing toy run at trunk and at head. Trunk sends no `traceparent`, so record that and gate the head verdict plus the `spanId` attribute on every call span. Save `calls.json`. Pass when the head verdict is pass and every call span has a `spanId`.
- [ ] Lane 2. Make the toy `app` log each request's `traceparent` header for this lane only. Save `app-log.txt`. Pass when every logged trace id equals `tracectx.TraceID` of the run id.
- [ ] Lane 3. Add a flow step that sets its own `traceparent`. Save `script-wins.json`. Pass when that call's `spanId` equals the span id the script sent.
- [ ] Lane 4. Add a flow step with a malformed `traceparent`. Save `malformed.json`. Pass when the step passes and its call span has no `spanId`.
- [ ] Lane 5. Run `bin/cockpit runs exec` on a kept run. Save `exec-traceparent.txt`. Pass when the exec calls use `tracectx.ExecTraceID`, not the run trace id.
- [ ] Lane 6. Open the flow screen's step requests. Save `step-requests.png`. Pass when the requests render as before.
- [ ] Lane 7. Run the seal check at head. Save `seal.json`. Pass when all static assertions pass and the probes stay sealed.
- [ ] Lane 8. Compare `schema/result.json` with a result written at head. Save `schema-check.txt`. Pass when the result validates and `schemaVersion` is still 2.
- [ ] Lane 9. Run a flow with more than 50 calls in one step. Save `cap.json`. Pass when the first 50 calls keep their own span ids and the cap behaves as before.
- [ ] Lane 10. Destroy the run and read its bundle. Save `bundle-calls.txt`. Pass when the frozen call spans keep `spanId`.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Mean duration of a toy call as recorded in `StepCall.DurationMs`, and suite stage wall time.
- [ ] Probe. Three toy runs at trunk and three at head, interleaved.
- [ ] Baseline. Record the trunk mean call duration first.
- [ ] Rule. Fail when the head mean call duration exceeds trunk by more than 1 ms or 5 percent, whichever is larger.

**Review gate.** None. PR2 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Review comments triaged.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends PR2 to the stack and the operator lands it.

## Give metrics series revisions and histograms (PR3)

**Depends on.** None.

**Files.**

- [ ] Edit `docs/signals.md` first. Extend the revision rule from events and spans to metrics with an `id`, and describe the histogram body.
- [ ] Edit `internal/contract/signal.go`, then run `go generate ./...` for `schema/` and `sdk/`.
- [ ] Edit `internal/contract/testdata/hostapi/1/` so it uses every new declared name.
- [ ] Edit `internal/signal/store.go`, `internal/signal/tail.go`, `internal/signal/unchanged.go` and `internal/signal/index.go`.
- [ ] Edit `internal/hub/screen/signalmodel.go` and `internal/hub/screen/signaltable.go`.
- [ ] Edit `internal/signal/revision_test.go` and give it a mutation record.

**Build.**

- [ ] `MetricBody` gains optional `Unit`, `Type` (`gauge`, `sum`, `histogram`, `summary`) and `Histogram *HistogramBody`, where `HistogramBody` holds `Count`, `Sum`, `Bounds` and `Counts`. `Value` stays required. For a histogram it holds the sum. The change only adds names, so `contract.HostAPI` stays at 1 and the fixture gains a use of each name.
- [ ] `record`, the tail and `unchangedWinner` stop excluding metrics. A metric with an `id` becomes a revision under the same key rule as an event, so a collector that re-sends a cumulative series every interval holds one record per series. A metric without an `id` stays append-only, so existing collectors keep their behavior.
- [ ] The screen's `versioned` picks metric winners by the same key, and the table shows `count` and `sum` for a histogram.

**You see.**

- [ ] A toy collector that emits the same metric id five times with rising `rev` shows one row with the last value in the signals table.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `revision_test.go` gains `TestAMetricWithAnIDKeepsOnlyItsHighestRevision` and `TestAMetricWithoutAnIDStaysAppendOnly`, and a mutation record whose results start with `red:`. Run `go test ./internal/signal`.
- [ ] `supersede_test.go` and `restart_test.go` gain metric cases, so replay supersedes the same records after a restart. Run `go test ./internal/signal`.
- [ ] `internal/contract` type-checks the host API fixture against the regenerated `sdk/`. Run `go test ./internal/contract`.
- [ ] `internal/hub/screen` gains `TestTheTableShowsTheWinningMetricRevisionAndAHistogramCount`. Run `go test ./internal/hub/screen`.
- [ ] Run `make generate-check` and `make verify`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `inherit-parent` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run the toy suite with its existing collectors at trunk and head. Save `collectors.json`. Pass when the head signal counts per collector equal trunk.
- [ ] Lane 2. Add a lane-only collector that emits one metric id with `rev` 1 to 5. Save `series.png`. Pass when the table shows one row with the fifth value.
- [ ] Lane 3. Same collector without an id. Save `no-id.json`. Pass when five records are stored.
- [ ] Lane 4. Emit a histogram metric. Save `histogram.png`. Pass when the row shows count and sum.
- [ ] Lane 5. Restart the hub after lane 2. Save `restart.json`. Pass when the series still reads back as one winner.
- [ ] Lane 6. Emit revisions across the freeze. Save `frozen.json`. Pass when the frozen copy holds the winner at the freeze and reads return the later winner.
- [ ] Lane 7. Emit a lower `rev` after a higher one. Save `lower-rev.json`. Pass when the higher revision stays the winner.
- [ ] Lane 8. Emit a metric whose attribute holds a planted credential value from the run tree. Save `masked.json`. Pass when the value reads back as `****` and the neighbouring plain attribute reads back unchanged.
- [ ] Lane 9. Write a collector against the regenerated `sdk/` with `bun` typecheck. Save `typecheck.txt`. Pass when the new fields type-check.
- [ ] Lane 10. Destroy the run and read the bundle. Save `bundle-metrics.txt`. Pass when the frozen metrics hold one winner per series.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. `BenchmarkIndexMemoryAfterThreeHundredThousandAppends` bytes per record, and append ns/op for a metric with an id.
- [ ] Probe. Run the benchmark with `-count=6` at trunk and head, interleaved.
- [ ] Baseline. Record the trunk bytes per record first.
- [ ] Rule. Fail when head bytes per record exceed 16, the bound `TestTheReopenedIndexHoldsAtMostSixteenBytesPerRecord` already holds, or when metric append ns/op exceeds trunk by more than 10 percent.

**Review gate.** None. PR3 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Review comments triaged.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends PR3 to the stack and the operator lands it.

## Build the OTLP receiver in the runner (PR4)

**Depends on.** PR3.

**Files.**

- [ ] Edit `docs/harness.md` first. Describe the `otlp` subcommand, its routes, its limits and its output lines.
- [ ] Edit `cmd/cockpit-runner/main.go` and its `usage` constant.
- [ ] Create `cmd/cockpit-runner/otlp.go`.
- [ ] Create `internal/runner/otlp/` with the server, the trace converter, the metric converter, the JSON id fix and the line writer, each under 300 lines, plus tests and synthetic fixtures in `internal/runner/otlp/testdata/`.
- [ ] Edit `go.mod` and `go.sum` to add `go.opentelemetry.io/proto/otlp` and `google.golang.org/protobuf` as direct dependencies.

**Build.**

- [ ] `cockpit-runner otlp <listen address>` follows the `edge` pattern. Bad arguments exit 2. It serves until the context ends and then returns 0.
- [ ] It accepts `POST /v1/traces` and `POST /v1/metrics` with `application/x-protobuf` or `application/json`, and `Content-Encoding: gzip`. Bodies over 4 MiB get 413. Other paths get 404. Logs are out of scope for this program.
- [ ] JSON decoding converts `traceId`, `spanId` and `parentSpanId` from hex to base64 before `protojson.Unmarshal`. OTLP JSON writes these ids as hex, and a plain `protojson` decode reads a 32-character hex string as valid base64 and returns 24 wrong bytes without an error.
- [ ] Each span becomes one output line with kind `span`, id `w3c:<span id>`, parent `w3c:<parent span id>`, trace id `w3c:<trace id>`, the service name from the resource attribute `service.name` in the attribute `service`, status `ok` or `error`, and the time source `container`. The hub maps the ids in PR6.
- [ ] Each metric data point becomes one output line with kind `metric`. Its id is `metric:` plus a digest of the service, the metric name and the sorted point attributes, and `rev` counts up per series. Delta temporality sums are accumulated to cumulative in the receiver, so the latest revision is always the total. Exponential histograms and summaries keep only count and sum.
- [ ] Every string passes `mask.New(mask.Options{Mode: mask.Runner})` before any truncation. Attribute values are cut to 1 KiB after redaction, and each output line stays under 64 KiB. The receiver has no private env file, so only the pattern rules apply here. The hub masks again with the value list on append.
- [ ] Output lines are JSON objects with a version field, `{"v":1,"signal":{...}}`, on stdout. Diagnostics go to stderr through `slog`.

**You see.**

- [ ] `cockpit-runner otlp :4318` prints one `{"v":1,...}` line per span and per metric point when a Go OTel SDK client exports to it, as in Appendix A.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `internal/runner/otlp` gains `TestAProtobufTraceExportBecomesOneLinePerSpan`, `TestAJSONTraceExportKeepsHexIDs`, `TestAGzipBodyDecodes`, `TestABodyOverTheLimitIsRefusedWith413`, `TestACumulativeSumKeepsOneSeriesWithRisingRevisions`, `TestADeltaSumIsAccumulated` and `TestAHistogramKeepsBoundsAndCounts`. Requests are built from the generated OTLP types, not from an SDK, so the license gate sees no new SDK dependency.
- [ ] A redaction test plants `PLANTED` behind an `authorization` attribute and asserts in one assertion that the plaintext is absent and `****` is present, next to a canary attribute that passes unchanged. The file carries a mutation record for the redaction and the truncation order.
- [ ] `cmd/cockpit-runner/main_test.go` gains `TestOTLPReturnsZeroWhenContextEnds` and an argument error case that exits 2.
- [ ] Run `go test ./internal/runner/otlp ./cmd/cockpit-runner` and `make verify`, including the license gate.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `inherit-parent` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run `cockpit-runner edge` and `cockpit-runner probe` at trunk and head. Trunk has no `otlp` subcommand, so record that and gate the head subcommand plus unchanged edge and probe behavior. Save `subcommands.txt`. Pass when edge and probe behave the same and `otlp` serves.
- [ ] Lane 2. Export traces from a Go OTel SDK client with `otlptracehttp`. Save `go-traces.txt`. Pass when every span arrives with its parent id.
- [ ] Lane 3. Export metrics from a Go OTel SDK client with a 1 s periodic reader. Save `go-metrics.txt`. Pass when each series keeps one id and `rev` rises per export.
- [ ] Lane 4. Post a hand-written OTLP/JSON trace with `curl`. Save `json-trace.txt`. Pass when the ids in the output equal the hex ids sent.
- [ ] Lane 5. Post a gzip protobuf body. Save `gzip.txt`. Pass when it decodes.
- [ ] Lane 6. Post a 5 MiB body. Save `too-big.txt`. Pass when the response is 413 and the process keeps serving.
- [ ] Lane 7. Post a span with a `PLANTED` bearer token attribute. Save `masked.txt`. Pass when the output holds `****` and not the plaintext.
- [ ] Lane 8. Post a span with a 10 KiB attribute. Save `truncated.txt`. Pass when the line stays under 64 KiB and the value is cut to 1 KiB.
- [ ] Lane 9. Send SIGTERM. Save `shutdown.txt`. Pass when the exit code is 0.
- [ ] Lane 10. Run the binary from the imported runner image as the runner user, read-only, with `cap_drop ALL`. Save `hardened.txt`. Pass when it serves under the `hardened()` settings.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Runner gz size per architecture from `make size`, receiver throughput in spans per second, and resident memory under load.
- [ ] Probe. Run `make size` at trunk and head. Run a benchmark that posts 1,000-span protobuf batches for 30 s at head, since trunk has no receiver.
- [ ] Baseline. Record the trunk gz sizes first.
- [ ] Rule. Fail when the arm64 gz grows by more than 3 MiB, when throughput is under 20,000 spans per second on the lane machine, or when resident memory exceeds 256 MiB. The size budget is a guess from Appendix A and the owner records the measured growth either way.

**Review gate.** None. PR4 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Review comments triaged.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends PR4 to the stack and the operator lands it.

## Add the OTLP harness role and the telemetry declaration (PR5)

**Depends on.** PR4.

**Files.**

- [ ] Edit `docs/sealing.md`, `docs/harness.md` and `docs/guide.md` first. The `roles.present` text names `otlp`. The guide documents the `telemetry` declaration and the variables the hub sets.
- [ ] Edit `internal/contract/declaration.go` and `internal/contract/declarationdocs.go`, then run `go generate ./...`.
- [ ] Edit `internal/compose/harness.go`, `internal/compose/render.go` and `internal/compose/model.go`, and raise `HarnessVersion`.
- [ ] Edit `internal/seal/static_test.go`, `internal/seal/fixture_normalize_test.go` and the seal fixture.
- [ ] Edit `internal/ports/docker/find_test.go` and `internal/hub/fakedocker/fake.go`.
- [ ] Edit `testdata/toy-project/.cockpit/project.yaml` and `testdata/toy-project/mock/`.

**Build.**

- [ ] The declaration gains `telemetry.otlp.services`, a list of compose service names. Validation refuses a name that is not a service of the model, and refuses the builder.
- [ ] `RoleOTLP = "otlp"` joins `HarnessServices`, so a consumer service named `otlp` is refused from now on. `Harness` emits the `otlp` service only when the declaration names at least one service. It uses `hardened(image(RoleOTLP))`, the command `otlp :4318`, the `inner` networks, no ports, no binds and no profile, so it runs from boot like the edge.
- [ ] For each named service, `Harness` adds `OTEL_EXPORTER_OTLP_ENDPOINT=http://otlp:4318`, `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`, `OTEL_METRIC_EXPORT_INTERVAL=2000`, `OTEL_BSP_SCHEDULE_DELAY=500` and `OTEL_TRACES_SAMPLER=parentbased_always_on`. The harness file comes after the consumer files, so these keys win.
- [ ] `requiredRoles` requires `otlp` only when the declaration names services. The existing network, mount, port and edge assertions already confine it. The static tests prove that with violating copies.
- [ ] The toy `mock` app, with no dependencies, reads `traceparent` on each request and posts one OTLP/JSON span per request with that parent and a counter and a histogram every 2 s to `OTEL_EXPORTER_OTLP_ENDPOINT`. The toy declaration names `app`.

**You see.**

- [ ] `docker compose ps` for a toy run lists `otlp` running with no published port, and the run's seal check passes all static assertions.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `internal/compose/harness_test.go` gains `TestTheOTLPRoleAppearsOnlyWhenTelemetryNamesAService` and `TestNamedServicesGetTheOTLPVariablesAndOthersDoNot`. Run `go test ./internal/compose`.
- [ ] `internal/compose/model_test.go` gains a deny case for a consumer service named `otlp` with an allow canary. Run `go test ./internal/compose`.
- [ ] `internal/contract` gains `TestATelemetryServiceThatIsNotInTheModelIsRefused`. Run `go test ./internal/contract`.
- [ ] `internal/seal/static_test.go` gains violating copies where `otlp` publishes a port, binds a path, or joins `egress`, each failing its assertion, plus the passing toy render, and new rows in `staticMutations`. Run `go test ./internal/seal`.
- [ ] `internal/ports/docker/find_test.go` proves the logs observer still skips the role-labelled `otlp` container. Run `go test ./internal/ports/docker`.
- [ ] Run `make seal-fixture`, then `make verify`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `inherit-parent` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run the toy consumer without the `telemetry` block at trunk and at head. Save `no-telemetry.txt`. Pass when both renders have no `otlp` service and both verdicts pass.
- [ ] Lane 2. Run the toy consumer with `telemetry.otlp.services: [app]`. Save `ps.txt`. Pass when `otlp` runs with no published port.
- [ ] Lane 3. Read the `otlp` container's stdout with `docker logs`. Save `otlp-stdout.txt`. Pass when it holds one span line per toy request and metric lines about every 2 s. Parent ids that match the runner's call spans need PR2 and are checked in PR6.
- [ ] Lane 4. Run `bin/cockpit run-env` with the unseal check. Save `unseal.txt`. Pass when the probes turn red as before and the sealed run passes.
- [ ] Lane 5. Exec into `otlp` and try `1.1.1.1:443`. Save `egress.txt`. Pass when the connection fails.
- [ ] Lane 6. Inspect `app`'s environment. Save `app-env.txt`. Pass when the five `OTEL_*` variables are set and the `db` service has none.
- [ ] Lane 7. Declare a service that does not exist. Save `bad-service.txt`. Pass when the submission is refused with the field named.
- [ ] Lane 8. Add a consumer service named `otlp`. Save `reserved-name.txt`. Pass when the submission is refused as reserved for the harness.
- [ ] Lane 9. Read the summary of a finished run. Save `summary.json`. Pass when it records the raised harness version.
- [ ] Lane 10. Stop and resume a kept run. Save `resume.txt`. Pass when `otlp` comes back up with the other services.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Boot stage duration and submit-to-verdict wall time for the toy run.
- [ ] Probe. Three toy runs at trunk and three at head with telemetry declared, interleaved.
- [ ] Baseline. Record the trunk median boot duration first.
- [ ] Rule. Fail when the head median boot duration exceeds trunk by more than 1 s, or the wall time by more than 3 percent.

**Review gate.** None. PR5 is not review-gated.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Review comments triaged.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends PR5 to the stack and the operator lands it.

## Ingest OTLP signals under the calls that caused them (PR6)

**Depends on.** PR2 and PR5.

**Files.**

- [ ] Edit `docs/signals.md` and `docs/screen.md` first. Add an "OTLP signals" section with the id mapping, the parent mapping, the quota and the drain.
- [ ] Create `internal/collect/observer/otlp.go` and `internal/collect/observer/otlp_test.go`.
- [ ] Edit `internal/ports/docker/find.go` to list a container by role.
- [ ] Edit `internal/hub/envops/collectports.go` to register the observer when telemetry is declared.
- [ ] Edit `internal/compose/collectors.go` to reserve the collector id `otlp`.
- [ ] Edit `internal/signal/caps.go` and its tests.
- [ ] Edit `internal/hub/pipeline/stage.go` for the drain and `internal/hub/pipeline/spans.go` for the parent mapping.
- [ ] Edit `internal/hub/screen/correlate.go` if step linking needs descendants of a step span.
- [ ] Edit `cmd/cockpit/e2e_run_test.go` and `internal/hub/pipeline/integration_test.go`.

**Build.**

- [ ] `observer.OTLP` finds the container labelled `io.cockpit.role=otlp`, follows its stdout with `docker logs --follow`, reuses `lineWriter` and the cursor with a 64 KiB line limit, and appends each `{"v":1}` line with `Stream.Append` under the collector `otlp`. The logs observer cannot do this, because `target()` drops role-labelled containers. A line that does not parse counts as dropped with a reason and never stops the follow.
- [ ] The observer maps the trace id `w3c:<tracectx.TraceID(run id)>` to the run id, so service spans join the run trace. An unknown trace id keeps its `w3c:` form and forms its own trace. Exec trace ids map to `<run id>/exec/<name>` while that exec runs.
- [ ] A new quota bucket keys OTLP records by their `service` attribute, with the reason `otlp-quota` and a default of 20,000 per service, replayed on restart like the other buckets.
- [ ] Drain at the end of `runSuite`, before it returns `Tested`. The observers are cancelled on the Tested to Reporting transition, so a drain in `reportStage` is too late. Wait until the `otlp` observer has appended nothing for 2.5 s, which is longer than the 2 s metric interval PR5 sets, or 10 s at most.
- [ ] In the report stage, after `runSpans`, build a map from each `StepCall.SpanID` to its `call:<flow>/<step>/<n>` id and append a revision of every `otlp` span whose parent is `w3c:<that span id>`, with the parent rewritten to the call span. Do this before `Freeze`, so the frozen copy holds the tree. The revision rule keeps the rewritten span as the winner.
- [ ] With `Options.NoCollection` there is no observer, and the docs say so.

**You see.**

- [ ] The trace view of a toy run shows each toy request's `app` span directly under its `call:` span, and the signals table shows one row per toy metric series with the final value.

**Verify, unit.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] `otlp_test.go` gains `TestAnOTLPLineBecomesASpanInTheRunTrace`, `TestAnUnknownTraceIDKeepsItsOwnTrace` and `TestABrokenLineIsCountedAndTheFollowContinues`, with a mutation record.
- [ ] `otlp_test.go` gains a redaction test at the observer call site where a span attribute holds a planted run credential value, asserting in one assertion that the plaintext is absent and `****` is present, next to a plain attribute that reads back unchanged.
- [ ] `internal/signal` caps tests gain `TestOTLPRecordsCountAgainstTheirServiceBucket` and a replay case. Run `go test ./internal/signal`.
- [ ] `internal/hub/pipeline` gains `TestServiceSpansAreReparentedUnderTheirCallBeforeTheFreeze` and `TestTheSuiteWaitsForTheOTLPStreamToGoQuiet` against the fake docker world. Run `go test ./internal/hub/pipeline`.
- [ ] The e2e test asserts that the frozen bundle holds at least one `otlp` span whose parent is a `call:` span and one metric series with one winner, and asserts how many it found. Run `COCKPIT_INTEGRATION=1 go test ./cmd/cockpit ./internal/hub/pipeline` on Docker Desktop.
- [ ] Run `make verify`.

**Verify, live.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked. Ten lanes on `inherit-parent` at the PR head, per the boot recipe.

- [ ] Lane 1. Regression lane against trunk. Run the toy consumer at trunk and at head. Trunk has no OTLP ingest, so record that and gate the head trace tree plus the passing verdict. Save `otlp-tree.png`. Pass when every toy call span has an `app` span child.
- [ ] Lane 2. Open the signals table filtered to `otlp` metrics. Save `metrics.png`. Pass when each series shows one row with the final counter value and the histogram count.
- [ ] Lane 3. Make one toy flow step fail. Save `failed-step.png`. Pass when the failed step's call shows its `app` span with error status.
- [ ] Lane 4. Destroy the run and read the bundle. Save `bundle-otlp.txt`. Pass when the frozen copy holds the reparented spans and the metric winners.
- [ ] Lane 5. Make the toy `app` send a span with a planted run credential in an attribute. Save `masked.png`. Pass when the screen and the bundle show `****` and never the plaintext.
- [ ] Lane 6. Make the toy `app` send 30,000 spans. Save `quota.json`. Pass when the read reports `otlp-quota` drops and the runner and pipeline spans are all kept.
- [ ] Lane 7. Restart the hub during the suite. Save `restart.json`. Pass when no OTLP record is duplicated and the follow resumes from its cursor.
- [ ] Lane 8. Make the toy `app` export with a 2 s delay after each request. Save `drain.txt`. Pass when the late spans are in the frozen copy, not only in the tail.
- [ ] Lane 9. Run with the declaration but stop the `otlp` container mid-suite. Save `otlp-down.txt`. Pass when the verdict still comes from the runner results alone and the read reports the gap.
- [ ] Lane 10. Run `bin/cockpit runs exec` on a kept run. Save `exec-otlp.png`. Pass when the exec's service spans sit under the exec trace.

**Verify, perf.** Tests alone are not sufficient verification. A PR is verified only when its unit, live, and perf boxes are all checked.

- [ ] Metric. Submit-to-verdict wall time, the drain time, and the hub's append rate for OTLP records.
- [ ] Probe. Three toy runs at trunk and three at head, interleaved, plus one head run with 30,000 spans for the append rate.
- [ ] Baseline. Record the trunk median wall time first.
- [ ] Rule. Fail when the head median wall time exceeds trunk by more than the 10 s drain cap plus 2 percent, when the median drain exceeds 4 s, or when the hub appends fewer than 5,000 OTLP records per second.

**Review gate.** The operator reviews before merge.

- [ ] Copy lane 1, lane 2 and lane 3 screenshots into `docs/plans/media/pr6-review-<slug>.png` on the lane machine, not in the commit.
- [ ] Record a 30 to 60 second video that opens a failed step and follows it into the service span. Save it as `pr6-review.mp4`.
- [ ] Post the screenshots and the video in chat. Stop at merge-ready. Wait for the operator's click.

**Merge.**

- [ ] Root's clean verdict at the exact head SHA.
- [ ] Review comments triaged.
- [ ] Rebased onto current trunk after the verdict, patch-id unchanged.
- [ ] The root appends PR6 to the stack and the operator lands it.

## Close the program

- [ ] Every box above is checked with its evidence.
- [ ] Reply to the operator with the report the execution playbook names, including the measured runner size growth from PR4 and the drain time from PR6.

## Appendix A. Prototype evidence

The prototypes were throwaway scripts in a session scratch directory. They were not committed, so there is no branch or SHA. Each finding below says how it was observed.

- The consumer services export no OTLP today. Measured on 2026-11-12 by listing the libraries of the local `wallet`, `policy-engine`, `travel-rule-gateway` and `webhook-manager` images and by reading the consumer's `backend/*/build.gradle.kts` at its latest commit. All four are Spring Boot 4 with `spring-boot-starter-actuator` and `micrometer-core`, and none has an OpenTelemetry, `micrometer-tracing` or OTLP registry library. The program therefore depends on the consumer change in Appendix D.
- A receiver built on the generated OTLP types decodes what a real SDK sends. Measured with a Go OTel SDK client using `otlptracehttp` with gzip and `otlpmetrichttp` with a 1 s reader against a prototype receiver. The span arrived as `application/x-protobuf` with `Content-Encoding: gzip`, its parent id equal to the span id in the incoming `traceparent`. A counter arrived as a `Sum` and a histogram as a `Histogram`. Both metrics arrived again on the next interval, which is why PR3 adds series revisions.
- Binary size. Measured with `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w"`. A bare `net/http` server was 5,439,648 bytes. The same server decoding with `go.opentelemetry.io/proto/otlp` v1.11.1 and `google.golang.org/protobuf` v1.36.12 was 12,124,320 bytes, with or without `protojson`. A decoder using only `protowire` was 6,095,008 bytes. The growth inside the runner, which already links `net/http`, is not measured, and PR4's perf box measures it.
- Spring Boot 4.1 behaviour, read from the Spring Boot 4.1.0 reference documentation. OTLP tracing needs `spring-boot-starter-opentelemetry`. Spring Boot samples 10 percent of traces by default with a parent-based sampler. It maps a subset of the standard `OTEL_*` variables to its properties, and a signal-specific endpoint wins over `OTEL_EXPORTER_OTLP_ENDPOINT`, which gets `/v1/traces` or `/v1/metrics` appended. Which variables are in the subset is not proven, so Appendix D sets the Spring properties too.

Unproven questions. The end-to-end export from a Spring Boot 4 service into the receiver has not been run. The runner size growth and the drain time on a real consumer are measured in PR4 and PR6.

## Appendix B. Alternatives rejected

- An OTel SDK inside the hub with an external exporter. The hub is a local tool with no backend to send to, the seal forbids service data from leaving the network, and the signal stream already has an OTel-shaped span model and a trace view.
- A third-party collector image as the receiver. Every harness role runs the imported runner image with no base image, and the license gate only covers Go modules, so a foreign image would bypass both.
- Reading the receiver through the logs observer. `target()` drops every role-labelled container, so the logs observer never sees it. A separate observer keeps the logs quota and the OTLP quota apart.
- A bind mount from the receiver to the run tree. `bindRoot` gives harness roles no binds, and opening one would weaken a seal assertion for no gain over stdout.
- One trace id per step. With the receiver in place, service spans attach to their call, so one trace per run matches the existing run trace and keeps the tree whole.
- Parenting service spans at read time in the screen through an alias attribute. That would make the frozen bundle depend on a screen rule. Report-time revisions keep the stream self-describing.
- A `protowire` decoder written by hand. It is about 6 MB smaller in the prototype, but it would hand-copy the OTLP field numbers. The generated types keep the schema owned upstream. The owner of PR4 may revisit this if the measured growth breaks the budget.
- Attaching the OpenTelemetry Java agent through `JAVA_TOOL_OPTIONS`. It would change the runtime under test, put Java knowledge in core, and ship a third-party jar into consumer containers.
- Keeping every metric point instead of series revisions. A cumulative series re-sent every 2 s would fill the quota with copies of the same total.

## Appendix C. Risks

- Spring Boot may not map all five `OTEL_*` variables (PR5). The owner of PR5 keeps the variables generic in core, and the consumer sets the Spring properties in its own overlay per Appendix D.
- The drain adds up to 10 s per run (PR6). The owner watches the median drain in the perf lane and lowers the quiet window if exports settle sooner.
- Parent-based sampling depends on the runner's sampled flag (PR2). Spans that a service starts on its own, such as scheduled jobs, are sampled at the service's own rate, and the guide says so.
- The runner gz grows by several MB (PR4). The owner records the measured growth and raises it with the operator if it exceeds the budget.
- A consumer service already named `otlp` stops passing validation (PR5). The release notes call it out.
- No `control-cli` or `control-ui` skill is available in Claude Code. Live lanes drive the CLI and the hub routes directly and the screen through a browser, as the boot recipe says.

## Appendix D. Links and reading list

Read before editing. `AGENTS.md`, `docs/signals.md` (runner spans, correlation keys, quotas), `docs/sealing.md` (static assertion table), `docs/harness.md` (roles), `docs/screen.md` (trace view, correlation), `docs/guide.md` (flows and collectors).

Route PR3 and PR6 through the pstack `how` skill before their first edit, and route the id and parent mapping of PR2 and PR6 through the `interrogate` skill before code-ready. Keep the decision trail per the `show-me-your-work` skill.

Consumer recipe for each Spring Boot 4 service the declaration names. This is guidance for the consumer repository, not cockpit code.

- Add `org.springframework.boot:spring-boot-starter-opentelemetry` for traces and `io.micrometer:micrometer-registry-otlp` for metrics.
- In the consumer's cockpit compose overlay, set `MANAGEMENT_OPENTELEMETRY_TRACING_EXPORT_OTLP_ENDPOINT=http://otlp:4318/v1/traces`, `MANAGEMENT_OTLP_METRICS_EXPORT_URL=http://otlp:4318/v1/metrics` and `MANAGEMENT_OTLP_METRICS_EXPORT_STEP=2s`, so the run works even where Spring does not map an `OTEL_*` variable.
- Leave sampling at its parent-based default, so every request the runner sends is sampled through its `traceparent` flag. Set `MANAGEMENT_TRACING_SAMPLING_PROBABILITY=1.0` only to also keep spans the service starts on its own.
- Keep the endpoints pointed at `otlp` inside the cockpit overlay only. Outside cockpit the services keep their own settings.
