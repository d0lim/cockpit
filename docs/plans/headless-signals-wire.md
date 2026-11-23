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
- [ ] Shared functions, hub callers migrated. Evidence: pending.
- [ ] Headless signal stream, stage spans and observers. Evidence: pending.
- [ ] `cockpit run --wire`. Evidence: pending.
- [ ] Action input `wire`. Evidence: pending.
- [ ] Mutations, each run in a scratch copy, each red. Evidence: pending.
- [ ] `make verify` in a clean copy. Evidence: pending.

## Left open

- A live run in GitHub Actions with `wire: true`, imported into a hub. Pending.
