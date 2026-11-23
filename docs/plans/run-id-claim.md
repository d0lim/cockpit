# Run id claim plan

Goal. No two runs on one Docker host ever get the same run id. That holds for a hub run and a `cockpit run`, and for two `cockpit run` processes started in the same second on the same commit. The change must not add GitHub Actions minutes.

## The problem

The run id is the compose project name, so two runs with one id share containers, networks and volumes. The hub names a run `run-<date>-<time>-<sha7>[-<n>]` from its own store when the run is submitted. `cockpit run` used the same shape and skipped the ids it could see: cockpit containers and networks on the host and the hub's run directories. Two windows stayed open.

- Hub after headless. `cockpit run` creates its project, and the hub accepts the same commit in the same second. The hub looks only at its store, so it gives out the same id. Closing this from the hub side would make every submission query Docker.
- Headless after headless. Two `cockpit run` processes read the host at the same moment, see the same free id, and both take it.

A GitHub Actions job gets its own machine, so neither window opens in CI. Both are local.

## Decisions

- The two id sources get disjoint shapes. A `cockpit run` id ends in `-h<n>` (`store.HeadlessRunID`), and `store.NewRunID` never makes one. A hub run and a headless run then cannot collide whatever the timing, and the hub needs no Docker query at submission.
- Headless runs claim an id atomically. The run directory moves from `os.MkdirTemp` to `os.Mkdir` of `<temp dir>/cockpit-<run id>`. Creating a directory fails when it already exists, so of two runs that pick the same `n` only one succeeds, and the other tries `n+1`. The claim is the run directory itself, so it has nothing extra to clean up. The existing cleanup removes it, `--keep` keeps it next to the kept project, and a refused run removes it.
- `cockpit run` keeps skipping the ids of cockpit containers and networks. That covers a kept run whose directory a person removed. It stops reading the hub's run directories, because hub ids can no longer match.
- `store.IsRunID` accepts `-h<n>`, so a hub on the same host can name a kept headless project.
- Not done: a lock or reservation file shared by the hub and `cockpit run`. The disjoint shapes make it unnecessary, and it would tie `cockpit run` to the hub's state directory.

## Actions minutes

The claim adds one `mkdir` to a step that already ran `docker ps` and `docker network ls`. It adds no step, image or download to a job. The action job on the toy project took 1 minute 40 seconds in CI before this change.

The minutes a consumer pays are the job's wall time. `docs/hub.md` now shows the guards that bound them: trigger on pull requests, `concurrency` with `cancel-in-progress` so a newer push cancels the older run, and `timeout-minutes` so a hung suite does not run to the six-hour default.

## Checklist

- [x] Docs first: `docs/sealing.md` (the headless naming and claim), `docs/hub.md` (the cost guards in the workflow example), `skill/cockpit/reading.md` (what `-h<n>` means). Evidence: the diff of this PR.
- [x] `store.HeadlessRunID` and the `-h<n>` branch of the run id pattern. Evidence: `TestTheHubNeverMakesAHeadlessRunIDEvenWhenEveryOtherIDIsTaken`, and the allow and deny cases in `TestIsRunIDAcceptsTheRunIDShapeAndRejectsPathsCaseAndOverlongSequences`.
- [x] `claimRunID` in `internal/headless/runid.go`, and the run directory that the claim created. Evidence: `TestTheRunIDSkipsIDsThatContainersNetworksOrClaimsHold`, `TestRunsThatClaimAtTheSameTimeNeverShareAnID` (32 concurrent claims, 32 distinct ids), `TestARunReleasesItsClaimWhetherItIsRefusedOrFinishes`.
- [x] Mutations, each run in a scratch copy, each red:
  - [x] `takenRunIDs` returns no ids → the run takes the id a container carries.
  - [x] `os.MkdirAll` replaces `os.Mkdir` → concurrent runs share an id.
  - [x] the refused run's `os.Remove` is dropped → its claim stays.
  - [x] `HeadlessRunID` drops the `h` → the hub makes an id of the headless shape.
- [x] `make verify` in a clean copy. Evidence: exit 0, the build and the size measurement at the end of its log.
- [x] A live `cockpit run` on `testdata/toy-project` with Docker Desktop prints a `-h1` id, passes, and leaves no `cockpit-run-*` directory or labelled resource. Evidence: run `run-20261123-131602-5bdb610-h1`, run directory `cockpit-run-20261123-131602-5bdb610-h1`, `verdict passed`, cleanup "0 labeled resources left; run directory removed", no `cockpit-run-*` entry left in the temp directory.
- [ ] CI green on the PR.

## Left open

Two `cockpit run` processes started at once on one machine still pick the same free port block, because the block is checked by a test bind and taken only when the edge starts. The later run then fails at boot with a bind error instead of joining the other run. The same claim by `os.Mkdir` would close it, as a follow-up.
