# Run id claim plan

Goal. No two runs on one Docker host ever get the same run id. That holds for a hub run and a `cockpit run`, and for two `cockpit run` processes started in the same second on the same commit. The change must not add GitHub Actions minutes.

## The problem

The run id is the compose project name, so two runs with one id share containers, networks and volumes. The hub names a run `run-<date>-<time>-<sha7>[-<n>]` from its own store when the run is submitted. `cockpit run` used the same shape and skipped the ids it could see: cockpit containers and networks on the host and the hub's run directories. Two windows stayed open.

- Hub after headless. `cockpit run` creates its project, and the hub accepts the same commit in the same second. The hub looks only at its store, so it gives out the same id. Closing this from the hub side would make every submission query Docker.
- Headless after headless. Two `cockpit run` processes read the host at the same moment, see the same free id, and both take it.

A GitHub Actions job gets its own machine, so neither window opens in CI. Both are local.

## Decisions

- The two id sources get disjoint shapes. A `cockpit run` id ends in `-h<n>` (`store.HeadlessRunID`), and `store.NewRunID` never makes one. A hub run and a headless run then cannot collide whatever the timing, and the hub needs no Docker query at submission.
- Headless runs claim an id atomically in Docker. After the runner image is ensured, the run creates a stopped container `cockpit-claim-<run id>` from it, with no network and the label `io.cockpit.claim`. Docker refuses a duplicate container name, so of two runs that pick the same `n` only one succeeds, and the other tries `n+1`. Any other `docker create` failure refuses the run. The run removes the claim container at its end, refused, failed, passed or kept.
- Why Docker and not the filesystem. The first version claimed by `os.Mkdir` of the run directory in the temporary directory. Review (Greptile on #19) pointed out that two runs with different `TMPDIR` values, such as an IDE and a terminal, or a run under `sudo`, never see each other's directories and can still share an id, and one run's cleanup can then tear down the other's project. The run id names a compose project in one Docker daemon, so that daemon is the namespace to claim in. A claim container costs no image pull, because the runner image is already there, and it never starts.
- `cockpit run` keeps skipping the ids of cockpit containers and networks. That covers a kept run, whose claim container is gone but whose project remains. It stops reading the hub's run directories, because hub ids can no longer match.
- `store.IsRunID` accepts `-h<n>`, so a hub on the same host can name a kept headless project.
- Not done: a lock or reservation file shared by the hub and `cockpit run`. The disjoint shapes make it unnecessary, and it would tie `cockpit run` to the hub's state directory.

## Actions minutes

The claim adds one `docker create` and one `docker rm` of a container that never starts, from an image the run already has, next to the `docker ps` and `docker network ls` the run already made. Together they take well under a second. It adds no job step, image or download. The action job on the toy project took 1 minute 40 seconds in CI before this change.

The minutes a consumer pays are the job's wall time. `docs/hub.md` now shows the guards that bound them: trigger on pull requests, `concurrency` with `cancel-in-progress` so a newer push cancels the older run, and `timeout-minutes` so a hung suite does not run to the six-hour default.

## Checklist

- [x] Docs first: `docs/sealing.md` (the headless naming and claim), `docs/hub.md` (the cost guards in the workflow example), `skill/cockpit/reading.md` (what `-h<n>` means). Evidence: the diff of this PR.
- [x] `store.HeadlessRunID` and the `-h<n>` branch of the run id pattern. Evidence: `TestTheHubNeverMakesAHeadlessRunIDEvenWhenEveryOtherIDIsTaken`, and the allow and deny cases in `TestIsRunIDAcceptsTheRunIDShapeAndRejectsPathsCaseAndOverlongSequences`.
- [x] `claimRunID` and `release` in `internal/headless/runid.go`. Evidence: `TestTheRunIDSkipsIDsThatContainersNetworksOrClaimsHold` (also: a create error other than a name conflict refuses after one try), `TestRunsThatClaimAtTheSameTimeNeverShareAnID` (32 concurrent claims against one name registry, 32 distinct ids), `TestARunReleasesItsClaimWhetherItIsRefusedOrFinishes` (one create and one `rm -f` of the claim for a passing run and for a run refused after its claim).
- [x] Docker's refusal text is what `claimRunID` matches. Evidence: a second `docker create --name` on Docker Desktop 29.9.2 answered `Conflict. The container name "/cockpit-claim-probe" is already in use`.
- [x] Mutations, each run in a scratch copy, each red:
  - [x] `takenRunIDs` returns no ids → the run takes the id a container carries.
  - [x] every `docker create` result counts as a claim → concurrent runs share an id.
  - [x] the `docker rm` in `release` is dropped → the claim container stays.
  - [x] `HeadlessRunID` drops the `h` → the hub makes an id of the headless shape.
- [x] `make verify` in a clean copy. Evidence: exit 0, the build and the size measurement at the end of its log.
- [ ] Two live `cockpit run` processes on `testdata/toy-project` with Docker Desktop, started in the same second with different `TMPDIR` values and different port bases, get different ids, both pass, and leave no claim container or labelled resource. Evidence: both started at 13:25:58 on commit `5fe50cd`, one with `TMPDIR=tmp1` and port base 22000, the other with `TMPDIR=tmp2` and port base 23000. They got `run-20261123-132558-5fe50cd-h1` and `-h2`, each `run id` stage took about 200 ms, both ended `verdict passed`, both printed `release run id ... removed`, and `docker ps -a --filter label=io.cockpit.claim` was empty afterwards.
- [ ] CI green on the PR.

## Left open

Two `cockpit run` processes started at once on one machine still pick the same free port block, because the block is checked by a test bind and taken only when the edge starts. The later run then fails at boot with a bind error instead of joining the other run. A claim container per port block would close it, as a follow-up.
