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

## Docker reconciliation

Reconciliation runs inside the tick: on the first tick after the hub starts and on the first tick after Docker answers again. While Docker does not answer, queued runs wait with `docker-unavailable` and a periodic tick keeps checking. Runs that an operation of this process is working on are skipped.

- A broken `state.json` is rebuilt from the run records: the queue in submit order, the leases from the slots of runs that hold an environment.
- A kept run takes the observed environment. A run that should be up but is stopped is only marked stopped and is not resumed. A run with no containers, networks or volumes is marked `lost`; its report bundle and record stay, and the rest of its run directory and its lease are released.
- An active run that the hub left behind first has its child process group killed (only when it started in the current boot) and its one-off containers removed, then closes as `error` with the notice "hub stopped". A run that was running or reporting goes through the report when the runner left its result file.
- A run left in an operation in progress has its leftovers cleared and the same operation done again.
- A queued run pinned to a host API this hub does not serve is refused (`done/absent`, verdict `error`) and leaves the queue.
- Labelled projects without a record are listed as orphans and never touched.

## Host API pinning

Each run records `cockpitVersion`, `hostApi` and, once the runner image is imported, `runnerImage` (tag and image ID). A submission whose declaration pins a host API outside the served set is refused with `REJECTED_HOST_API` and creates no run. A kept run pinned to a host API the hub no longer serves is listed and can be destroyed, but resume and exec are refused with the same notice: the unknown version, the served set and the hub version.

## Coexistence with the previous cockpit

The two hubs can run on one host without sharing a lock, ports or reconciliation.

| Item | This hub | Previous hub |
|---|---|---|
| Listen address | `127.0.0.1:7710` (`COCKPIT_PORT` changes the port only) | `127.0.0.1:7700` |
| Labels | `io.cockpit.managed`, `io.cockpit.run-id` | its own label names |
| Port blocks | 22000-22799, 8 slots of 100 | 21000-21799 |
| State directory | `~/.local/state/cockpit` (under `XDG_STATE_HOME` when set, `COCKPIT_HOME` overrides) | `~/.cockpit` |
| Lock file | `hub.lock` in the state directory | `hub.lock` in its own directory |

- The hub refuses to start when the state directory holds the previous layout: a `state.json` with `schemaVersion: 1`, or a `runs/*/meta.json` with a `harness` field. The message names the directory and says to move it away or to point `COCKPIT_HOME` at an empty directory. This hub writes `schemaVersion: 2` and its run records carry no `harness` field.
- A port block layout is checked before any lease: it must not overlap the previous hub's block, the hub listen port, or the 17 ports of the manual stack, and every offset must fit inside the block width.
- `hub.lock` is created exclusively with mode 600 and holds the owner pid, port and start time. A lock held by a live pid refuses the start with that hub's address. A lock held by a dead pid is taken over. A lock that cannot be read is never taken over; the message names the file to delete.
