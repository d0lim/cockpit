# Lifecycle

This page holds the run states and the transition table, and the rules of the queue, the kept pool, the running cap, stop candidates, Docker reconciliation, host API pinning and coexistence with the previous cockpit. The code is in `internal/hub/{lifecycle,store,pool,lease,lock,reconcile,ticker,scheduler}`. When this page and the code disagree, this page is fixed first and the code follows. `TestLifecycleDocumentTableMatchesTheTransitionTableBothWays` reads the transition table below and compares it with `lifecycle.Transitions` in both directions.

## Three axes and two marks

A run's state is three independent axes and an optional mark.

- **Phase**: `queued`, the six active phases `preparing` · `building` · `sealing` · `booting` · `testing` · `reporting`, the three operations in progress `stopping` · `resuming` · `destroying`, and `done`.
- **Environment**: `absent` (never existed or vanished), `up`, `stopped`, `destroyed` (removed by a person or by the kept pool).
- **Verdict**: `null` until the run ends, then `passed`, `failed`, `error` or `aborted`. Once set, a verdict never changes.
- **Mark**: `lost` is a run whose record remains but whose containers, networks and volumes are all gone. `orphan` is a compose project that carries the cockpit label but has no record. A mark wins over the phase.

The table key is the phase, or `done/<environment>` for a finished run, or the mark. `new` is a run that has not been submitted. The labels shown by the screen and the CLI come from `lifecycle.Label`.

| Key | Label |
|---|---|
| `queued` | Queued |
| `preparing` | Preparing |
| `building` | Building |
| `sealing` | Checking the seal |
| `booting` | Booting |
| `testing` | Running |
| `reporting` | Reporting |
| `done/up` | Done (up) |
| `done/stopped` | Stopped |
| `done/absent` | Closed (no environment) |
| `done/destroyed` | Destroyed |
| `stopping` | Stopping |
| `resuming` | Resuming |
| `destroying` | Destroying |
| `lost` | Lost |
| `orphan` | Orphan |

## Transition table

Each row is one allowed cell. `-` keeps the axis as it is. `observed:<what>` is reconciliation seeing the environment `up`, `stopped` (including only volumes left, which needs recreation on resume) or `gone`. Any cell not listed is refused with a conflict (HTTP 409) whose reason names the current state label and the event. `done/absent`, `done/destroyed`, `lost` and `orphan` accept no event.

The caller records the new state before it does the work. An operation that fails stays in its in-progress phase with a notice, and the next tick or the reconciliation at the next start does the same work again.

| State | Event | Phase | Env | Verdict | Mark |
|---|---|---|---|---|---|
| new | submit | queued | absent | - | - |
| queued | tick-start | preparing | - | - | - |
| queued | destroy | done | destroyed | aborted | - |
| queued | reject | done | absent | error | - |
| preparing | prepared | building | - | - | - |
| preparing | destroy | destroying | - | aborted | - |
| preparing | fail | done | absent | failure | - |
| preparing | abort | done | absent | aborted | - |
| preparing | observed:up | done | absent | error | - |
| preparing | observed:stopped | done | absent | error | - |
| preparing | observed:gone | done | absent | error | - |
| building | built | sealing | - | - | - |
| building | destroy | destroying | - | aborted | - |
| building | fail | done | absent | failure | - |
| building | abort | done | absent | aborted | - |
| building | observed:up | done | absent | error | - |
| building | observed:stopped | done | absent | error | - |
| building | observed:gone | done | absent | error | - |
| sealing | sealed | booting | - | - | - |
| sealing | destroy | destroying | - | aborted | - |
| sealing | fail | done | absent | failure | - |
| sealing | abort | done | absent | aborted | - |
| sealing | observed:up | done | absent | error | - |
| sealing | observed:stopped | done | absent | error | - |
| sealing | observed:gone | done | absent | error | - |
| booting | booted | testing | up | - | - |
| booting | destroy | destroying | - | aborted | - |
| booting | fail | done | stopped | failure | - |
| booting | abort | done | stopped | aborted | - |
| booting | observed:up | done | stopped | error | - |
| booting | observed:stopped | done | stopped | error | - |
| booting | observed:gone | done | absent | error | - |
| testing | tested | reporting | - | - | - |
| testing | destroy | destroying | - | aborted | - |
| testing | fail | reporting | - | failure | - |
| testing | abort | reporting | - | aborted | - |
| testing | observed:up | reporting | - | error | - |
| testing | observed:stopped | reporting | stopped | error | - |
| testing | observed:gone | reporting | absent | error | - |
| reporting | reported | done | - | report | - |
| reporting | destroy | destroying | - | aborted | - |
| reporting | fail | done | - | failure | - |
| reporting | abort | - | - | aborted | - |
| reporting | observed:up | - | - | error | - |
| reporting | observed:stopped | - | stopped | error | - |
| reporting | observed:gone | - | absent | error | - |
| stopping | stopped | done | stopped | - | - |
| resuming | resumed | done | up | - | - |
| resuming | resume-failed | done | stopped | - | - |
| destroying | destroyed | done | destroyed | - | - |
| done/up | stop | stopping | - | - | - |
| done/up | destroy | destroying | - | - | - |
| done/up | observed:up | - | - | - | - |
| done/up | observed:stopped | - | stopped | - | - |
| done/up | observed:gone | - | absent | - | lost |
| done/stopped | resume | resuming | - | - | - |
| done/stopped | destroy | destroying | - | - | - |
| done/stopped | observed:up | - | up | - | - |
| done/stopped | observed:stopped | - | - | - | - |
| done/stopped | observed:gone | - | absent | - | lost |

## Verdict rules

A cell decides the verdict only while the verdict is still `null`, so the report of an aborted run never turns it into `passed`.

- `aborted` and `error` set that value.
- `failure` sets `failed` when the event carries `failed`, and `error` otherwise.
- `report` sets the verdict the event carries, and `error` when it carries none.

## Queue

- A submission joins the queue at the end. Each host has one active run.
- A submission of the same commit and suite as a run that is still queued returns that run instead of a new one. `new` (the CLI `--new`) always queues a new run.
- The queue holds at most 10 runs (`QUEUE_FULL` beyond that).
- When the head cannot start, every queued run records the wait reason: `active-run`, `in-progress`, `pool-over`, `pool-all-kept`, `docker-unavailable` or `no-port-block`.
- Destroying a queued run removes it from the queue and leaves only its record (`done/destroyed`, verdict `aborted`).

## Kept pool

The kept pool holds the finished runs whose environment remains (`up` or `stopped`, including runs being stopped or resumed). A run being destroyed is already leaving and is not counted.

- The pool holds at most five.
- When it holds more, one run is destroyed per pass: the unkept run with the oldest finish time, never the run that finished last. If every other run has keep on, nothing is destroyed and the pool stays at six; the queue then waits with `pool-over` until a person destroys one.
- With five runs that all have keep on, the queue waits with `pool-all-kept`.
- Keep cannot change on a run whose environment is gone or leaving.

## Running cap

The running count is the kept runs that are up or resuming, plus one for the active share (an active run, or a queue head that can start now), plus every running compose project on the host that is not one of this hub's runs. A project is this hub's when its containers or networks carry the label `io.cockpit.run-id`; a labelled project without a record is an orphan, and a running orphan counts too. A project without the label counts as one whatever ports it publishes; the hub never recognizes other stacks by port numbers.

- The cap defaults to 3 (`COCKPIT_UP_CAP`).
- Before the head starts, kept runs that are up are stopped, oldest last-up time first, until the count fits the cap (stop reason `cap`).
- A resume that would exceed the cap is refused with `CONFLICT_CAP` and the stop candidates: the kept runs that are up, oldest last-up time first.
- When a run finishes, its environment is stopped right away (stop reason `done`) and comes back only when a person resumes it. `COCKPIT_STOP_ON_DONE=0` (or `false`, `off`) leaves finished runs up and lets the cap stop them.

The pool size, the queue cap, the running cap and the port block layout are core defaults that the hub configuration replaces.

## Pipeline

A queued run that gets a port block goes through the stages one after another. Each stage ends with one event; an error before the report closes the run with the failed stage, the masked notice and the last 50 masked lines (lines that are only a declared secret file name are left out).

| Stage | Work | Limit |
| --- | --- | --- |
| prepare | import the runner image and pin its tag and ID, extract the commit, generate the environment, write the run files, check the static seal | one-shot calls 2 minutes |
| build | the builder and `compose build`, skipped when every built image is already present | 60 minutes |
| seal check | the probes and the positive control | 2 minutes |
| boot | `compose up` and wait until the expected services are ready (see **Readiness**) | 10 minutes |
| run | the runner | 20 minutes |
| report | judge the run and assemble the bundle with `internal/report` | none |

- **Acceptance**: a submission must come from an allowed repository, name a commit, have no uncommitted change under `inputs.dirty`, carry a declaration that reads, pin a served host API and name a suite in `scripts.suites` (or there is only one). The run records the repository's git common directory so the pipeline can read the commit again.
- **Readiness**: an expected service is ready when its container runs and, if it has a healthcheck, is healthy. A one-shot service is also ready when it exited with code 0. A service is one-shot when another service in the rendered config depends on it with `condition: service_completed_successfully`, or when its `restart` is `no`. A service with no `restart` is long-running, like one with any restart policy. The rule is `compose.OneShots`, read from the `compose config` the static seal already rendered. A one-shot service that exited non-zero or died fails the boot with the notice `one-shot service <name> failed (<state>, exit code <code>)`. A long-running service that exited, with any code, fails it with `<name> stopped (<state>, exit code <code>)`. A missing container waits until the limit.
- **Build skip**: a service with a `build` section is cached when its rendered image tag is one of the run's image keys and `docker image inspect` finds that image. The keys are content addresses of the image inputs, so a present image is the image this commit would build. When every built service is cached, the build stage runs neither the builder nor `compose build` and says `build skipped: all <n> images present`; one missing image, a tag that is not a key, or a built service without an image name builds as before.
- **Runner call**: `compose run -d -T --no-deps runner run <suite>`, then `docker wait` under the limit, `docker logs`, `docker inspect` for `OOMKilled` and `docker rm`. A runner over its limit, or one whose run is aborted, is force-removed. Every runner line goes through the run redactor before it reaches an event, the tail or an exec answer.
- **Report**: the report stage reads `work/e2e` with `report.ReadRunnerFiles`, the suite sources the manifest points to from `work/scripts`, the seal verdict from `seal.json`, the boot verdict and the runner outcome (exit code, OOM kill, masked lines) of this pipeline, and calls `report.Assemble` with the run masker, the run's signal stream and a meta patch. The verdict comes from the 15 rules in [verdict](verdict.md). A run the hub reports after a restart has no boot verdict or runner outcome; its verdict was set before the report and wins. The summary's `harness` is `compose.HarnessVersion` (5, the generated harness after the previous cockpit's 4) and `hubCommit` is the hub binary's VCS revision, or its version when the build carries none.
- **Runner identity**: a manifest whose runner identity differs from the pin sets the verdict to `error` before the report, and the notice names both values. A development build whose version carries no revision (`dev`) matches any `dev+<revision>`; two revisions or two releases must be equal.
- **Discarded bundle**: when the rescan finds a value left in the bundle, `report/` is not created, the meta gets `redaction: failed` with the paths inside the bundle, and the run's signal reads are refused.
- **Settling**: a failure in build or seal check brings the project down. A failure before an environment exists removes `src`, `work`, `env` and `build` and releases the lease. A boot failure stops the environment and keeps it. `build/` is removed after every pipeline.

## Collector supervision

The hub runs the run's collectors and its two observers (`logs` for container log lines, `services` for `docker compose ps` states) under `internal/collect`. Every transition the run store applies reaches the hub's observer after the record is written, and the hub maps it with `lifecycle.HookFor`: it arms after `booted`, `resumed`, a reconciled `up`, and `reported` when the environment stays up (a kept run keeps collecting live after its report, cause `kept`), and drains on entering `reporting`, `stopping` or `destroying`, on abort or failure, and when the environment is lost. The hook runs before the call that applied the transition returns, so compose stop, down and the report start only after the drain. A run whose environment ends (absent, destroyed or lost) is forgotten and its signal stream closed. Each `ctx.docker.exec` call of a collector, allowed or denied, leaves one `docker exec` line in the hub log with the run, the service, the masked argv and the outcome. The hub writes every log line, the API included, through the logger it was started with and never through the process default `slog` logger, so the hub log has one format.

- **Discovery**: at prepare, each `<scripts.collectors>/<id>.ts` in the extracted scripts is bundled with `worker.Bundle`. Its default export is evaluated once in a port-less engine to read `id` and `intervalMs`: the id must equal the file name, and `intervalMs` is absent (collected only at the report) or a whole number from 1000 to 3600000. An error fails the prepare stage. A hub that restarts discovers again when it re-arms.
- **Ports**: every collector may use `docker`, `signal` and `state`, `http` when the declaration has ports, and `sql` when it has databases. `http` reaches only `http://<port name>/…` on the run's block. A database connects to `127.0.0.1` on the run's port for the offset its `port` names (see harness.md, SQL port); a database whose port has no published port in the run refuses the run's sql port. Its root password is read from the dotenv secret sources. Collector docker calls do not touch the run's `pgid`.
- **Report**: the report stage asks the supervisor for a report-time collection (`skipped` when the environment is not up). Each collector becomes a summary stage named by its id, and the supervisor's unread records go to `notProven.unread` and `not-proven.md`. The report, the collectors and the API take the run's signal stream from the hub's registry, so the frozen segment and live reads hold what the collectors and observers emitted.
- **Off**: `Options.NoCollection` turns supervision off; collector stages are then absent from the summary.

## Runner image pin

The run pins the runner tag and the image ID that `docker import` returned. Before the runner runs, before resume and before exec, the hub checks that the tag still points at that ID. If the tag points elsewhere and the pinned image still exists, the tag is pointed back. If the pinned image is gone, the hub imports again and pins the new ID; a hub that embeds a different runner refuses instead.

## Process groups

Every docker call runs in its own process group. Cancelling a call sends SIGTERM to the group and SIGKILL after the grace period, so compose children never outlive it. While a call runs, the run record holds its `pgid`; it is cleared when the call ends. After a restart, reconciliation kills a recorded group only when it started in the current boot, and never the hub's own group, init or anything at or below 1.

## Kept environments

- **Stop**: `compose stop`. **Destroy**: `compose down -v --remove-orphans` after removing one-off containers, then `src`, `work`, `env` and `build` go and the lease is released; once the run is recorded as destroyed, its live signal stream (`signals/`) is closed and removed, and a later signal read answers not found without creating it again. The record, the report bundle (with its frozen signal copy) and `seal.json` stay.
- **Resume**: refuse when a port of the block is taken (the block is not leased again), check the runner pin, then start the resident services in dependency order, waiting for healthy dependencies through one-shot services. The services resume does not start are the one-shots of `compose.OneShots` and the services under a profile, which only start when their profile is named. When containers or networks are gone, the environment is created again without a build. Readiness is the same as boot, with the one-shot services read from a fresh `compose config`, so a one-shot that completed before the stop stays completed. Any failure stops the environment and closes the resume as failed with the reason.
- **Exec**: refused with 409 unless the run is kept and up and no operation is in progress (a stopped run is told to resume first), with 400 for an id outside `^[a-z][a-z0-9-]{1,40}$`, with 404 for a flow the run's manifest does not have, and with 409 for a once flow that already ran in this environment or a once prerequisite that has not passed. A passing call runs `run <suite> --flow <id>` with the pinned image and `E2E_EXEC_OUT=/work/e2e/exec/<time>-<id>.json`, and reads only that file back.

## Runner files

The hub reads `manifest.json`, `result.json`, `ledger.json` and exec results from `work/e2e` through the run directory root: the path must stay inside, the entry must be a regular file (checked with lstat and opened with `O_NOFOLLOW`), and it must be at most 4 MiB. A missing ledger counts as empty.

## Docker reconciliation

Reconciliation runs inside the tick: on the first tick after the hub starts and on the first tick after Docker answers again. While Docker does not answer, queued runs wait with `docker-unavailable` and a periodic tick keeps checking. Runs that an operation of this process is working on are skipped.

- A broken `state.json` is rebuilt from the run records: the queue in submit order, the leases from the slots of runs that hold an environment.
- A kept run takes the observed environment. A run that should be up but is stopped is only marked stopped and is not resumed. A run with no containers, networks or volumes is marked `lost`; its report bundle and record stay, and the rest of its run directory and its lease are released.
- An active run that the hub left behind first has its child process group killed (only when it started in the current boot) and its one-off containers removed, then closes as `error` with the notice "hub stopped". A run that was running or reporting goes through the report when the runner left its result file.
- A run left in an operation in progress has its leftovers cleared and the same operation done again. The tick does this only for a run with no operation running in this hub: stop, resume and destroy mark the run busy before they apply their event and keep it busy until they return, so a tick that lands between the event and the docker work never does the operation a second time. A second command on a run whose operation is in progress is refused with 409 and leaves the first operation's busy mark in place.
- A queued run pinned to a host API this hub does not serve is refused (`done/absent`, verdict `error`) and leaves the queue.
- Labelled projects without a record are listed as orphans and never touched.

## Host API pinning

Each run records `cockpitVersion`, `hostApi` and, once the runner image is imported, `runnerImage` (tag and image ID). A submission whose declaration pins a host API outside the served set is refused with `REJECTED_HOST_API` and creates no run. A kept run pinned to a host API the hub no longer serves is listed and can be destroyed, but resume and exec are refused with the same notice: the unknown version, the served set and the hub version.

## Coexistence with the previous cockpit

The two hubs can run on one host without sharing a lock, ports or reconciliation. This hub's own defaults are:

| Item | Default |
|---|---|
| Listen address | `127.0.0.1:7710` (`COCKPIT_PORT` changes the port only) |
| Labels | `io.cockpit.managed`, `io.cockpit.run-id` |
| Port blocks | 22000-22799, 8 slots of 100 |
| State directory | `~/.local/state/cockpit` (under `XDG_STATE_HOME` when set, `COCKPIT_HOME` overrides) |
| Lock file | `hub.lock` in the state directory |

- The hub refuses to start when the state directory holds the previous layout: a `state.json` with `schemaVersion: 1`, or a `runs/*/meta.json` with a `harness` field. The message names the directory and says to move it away or to point `COCKPIT_HOME` at an empty directory. This hub writes `schemaVersion: 2` and its run records carry no `harness` field.
- A port block layout is checked before any lease. Every offset must fit inside the block width, the blocks must stay within 1024-49151, and the blocks must not hold the hub listen port. Other ports to keep clear (another hub's blocks, a manual stack, any service on the host) are not core knowledge: the hub configuration lists them as reserved ranges (`reservedPorts`), and the layout refuses to overlap any of them.
- `hub.lock` is created exclusively with mode 600 and holds the owner pid, port and start time. A lock held by a live pid refuses the start with that hub's address. A lock held by a dead pid is taken over. A lock that cannot be read is never taken over; the message names the file to delete.
- Credential file names that run records must never contain are `.env` (and `.env.*`), `*.private.env.json` (the runner's private env) and `*.pem`. The caller adds further names from the project declaration's secret sources.
