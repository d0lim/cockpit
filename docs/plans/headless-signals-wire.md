# Headless signals and wire plan

Goal. A run that `cockpit run` makes in GitHub Actions, exported as a bundle archive and imported into a hub, shows the Signals trace and the Wire sequence like a run the hub made.

## The problem

`cockpit run` opened no signal stream, ran no observers and never recorded wire. Its bundle had no `report/signals/stream.jsonl` and no `report/wire/`, so an imported Actions run showed an empty signals page and no Wire screen. The hub's code for these lived behind its store: the report spans took an `envops.ReportInput`, the observers were built inside the collector registration, and `envops.RunWire` reached the hub's `Ops` for its docker client, its masking and its stream.

## Decisions

- No new domain types. The headless run owns what a hub run owns: a `*signal.Stream` in its run directory, `store.Meta.Stages` filled with `StartStage` and `EndStage`, the runner exit time, and an `envops.RunWire` for a recording run.
- The shared parts move behind store-free functions that the hub and `cockpit run` both call, and the hub callers move in the same change: `pipeline.AssembleReport` (stage and runner spans, the reparent, the `suite` take, the freeze, `Assemble`, `Attach`), `pipeline.StageOutcome`, `pipeline.WireRefusal`, `envops.NewRunObservers`, and an `envops.RunWire` that holds its docker client, mask, stream and logger instead of `*Ops`.
- Signals are always on. The observers run through a `collect.Supervisor` that holds no collectors, from the end of the boot stage until the suite ends, because the supervisor already retries an observer that stops and bounds the wait for it to stop. A run config without collectors needs no port factory.
- The stream and the observers read the run masker through an atomic pointer that the run sets after each refresh, because the observers append from their own goroutines while the run stage refreshes the masker.
- Wire recording is opt-in: `cockpit run --wire`, and the action input `wire`, default `false`. A commit without `wire.routes` refuses `--wire` in the declaration step, before the run claims an id.
- Collector scripts stay on the hub. The headless path starts no script host.

## Checklist

- [x] Docs first: `docs/sealing.md` (headless path, usage), `docs/verdict.md` (the shared report function), `docs/export.md` (an imported headless run has signals), `docs/wire.md` (`cockpit run --wire`), `docs/hub.md` (action input `wire`), `docs/signals.md` (headless stream and stage spans). Evidence: the first commit of this branch.
- [x] Shared functions, hub callers migrated. Evidence: the two refactor commits. The hub's `DefaultReport`, `endingStage`, `Accept`, `runConfig`, `DrainOTLP` and `RunWire` now call `AssembleReport`, `StageOutcome`, `WireRefusal`, `NewRunObservers`, `SettleOTLP` and `OTLPDrainLine`, and every hub, collect and headless test passed unchanged but for the moved `reportSpans` call. `TestARunOfObserversAloneRegistersWithoutAPortFactoryAndOneWithCollectorsDoesNot` covers the port factory rule.
- [x] Headless signal stream, stage spans and observers. Evidence: `internal/headless/signals_test.go`. The bundle of a passing run holds the `pipeline`, five `stage:*`, `run`, `flow:` and `step:` spans under the run id, parents first and all `ok`, the container log lines with `****` in place of a planted value, and the service rows; `signals` is `ok`. OTLP lines that arrive while the runner refreshes the masker are frozen and the runner line names the drain. The parity test now requires the hub and `cockpit run` to agree on `signals`, with a canary that both froze a stream. With the masker read straight from `r.env.Masker`, `go test -race` reported a data race between `RefreshMasker` and an OTLP append, so the run publishes its masker through an atomic pointer.
- [x] `cockpit run --wire`. Evidence: `internal/headless/wire_test.go`. A recording run on a commit with routes copies `wire/suite/index.json` and `calls.jsonl` with `****` in place of a planted value, the archive carries both under `report/wire/suite/`, `meta.json` says `"wire": true` and `summary.wire` is `taken` with two calls. `--wire` on a commit without routes exits 3 with the hub's refusal text and runs no `docker create` and no compose command. A kept run without `--wire` on a commit with routes has no `cockpit-wire` service in its harness and no `COCKPIT_WIRE_` variable in `run.env`. The parity test gained a recording case in which the hub and `cockpit run` write the same `summary.wire`. The headless integration test now also requires `signals` to be `ok` and the `pipeline`, `stage:suite` and `run` spans in the frozen stream.
- [ ] Action input `wire`. Evidence: pending.
- [ ] Mutations, each run in a scratch copy, each red. Evidence: pending.
- [ ] `make verify` in a clean copy. Evidence: pending.

## Left open

- A live run in GitHub Actions with `wire: true`, imported into a hub. Pending.
