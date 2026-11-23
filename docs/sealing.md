# Sealing

This page holds the five seals, the static assertion table and the rules of the sealed-network probe. The code is in `internal/seal` (assertions, probes, verdict, `seal.json`), `internal/compose` (harness, compose calls) and `internal/headless` (the headless path, `cockpit run`). When this page and the code disagree, this page is fixed first and the code follows. `TestSealingDocumentTableMatchesTheStaticAssertionsBothWays` reads the assertion table below and compares it with the names `seal.Static` returns, in order.

## The five seals

| Seal | What holds it |
| --- | --- |
| Fresh state | Every run gets a new compose project named after the run id, with its own volumes, networks and run directory. Credentials are generated inside the run tree by `env.command` in a networkless one-shot container. Destruction removes the volumes (`down -v`). |
| Isolation between sessions | Each run leases its own port block, its own project name and labels (`io.cockpit.managed`, `io.cockpit.run-id`), and its own run directory opened through `os.Root`. Edge ports bind `127.0.0.1` only. |
| Pinned commit | The source, the suite, the declaration and the consumer overlays come from the git objects of one commit and of the submodule commits it pins, extracted twice (`src/`, `build/`) and checked against each repository's file count in the tree. The harness and the runner come from the cockpit version. |
| Independence from host tools | The hub runs no consumer code on the host: environment generation, building, probing and the suite all run in containers, and every hub git call is hardened against repository configuration. |
| Zero external communication at runtime | At network level `sealed`: the default network and every project network are internal. Among long-running services only the edge joins `egress`, and it proxies only to service names of the project on declared offset ports. The static assertions prove the configuration, and the probes prove the network. At level `open` this seal is not held; see [Network level](#network-level). |

## Network level

The declaration's `network` field picks the level of the fifth seal: `sealed` (the default when the field is absent) or `open`. The level changes three things and nothing else:

| | `sealed` | `open` |
| --- | --- | --- |
| Harness | `default` and every project network are `internal` | the harness does not set `internal`, so project networks route out through Docker's bridge |
| Static assertions | all 25 | the 24 other than `networks.internal`; `services.networks` still confines every service to the networks of its role |
| Probes | the four sealed attempts and their control | not run, because an open network is expected to reach out |

The other four seals hold at both levels: fresh state, isolation between sessions (host ports only on the edge, only on `127.0.0.1`), the pinned commit and independence from host tools, with every privilege, mount, path and build assertion. `seal.json` records the level as `network`. The report records it in `summary.json` as `network`, and a run at `open` adds a `notProven.declared` line saying that runtime isolation was not proven. Use `open` for a stack that must call a real outside service; a stack whose outside calls can be faked keeps `sealed`.

## Static assertions

The hub renders the run's configuration once with `docker compose ... --profile '*' config --format json` and runs these 25 assertions on it in this order, before any image is built or any service starts. Every assertion runs even after another fails. A run fails the seal when any assertion fails. The results carry only service, network, port and path names, because the rendered configuration holds credential values.

| # | Name | Expectation |
| --- | --- | --- |
| 1 | `roles.present` | Exactly one service holds each harness role (`cockpit-edge`, `cockpit-runner`, `cockpit-probe`, `cockpit-probe-control`, `cockpit-otlp` when `telemetry.otlp.services` names a service, `cockpit-wire` when the run records, and `builder` when declared), and every service carries the run labels. Without this, an empty or partial render would pass the rest vacuously. |
| 2 | `files.overlays` | No compose file the render reads (the declared files, every file they include, and the harness) matches an entry of `compose.forbidden`. |
| 3 | `networks.set` | `default` and `egress` exist, and no network is external. |
| 4 | `networks.internal` | Every network other than `egress` is internal. Not run at network level `open`. |
| 5 | `services.networks` | The edge joins every project network and `egress`; the builder and `cockpit-probe-control` join only `egress`; every other service joins only project networks. A project network is any network other than `egress` that is not external. |
| 6 | `services.ports` | Host ports appear only on the edge. |
| 7 | `services.network-mode` | `network_mode` appears nowhere. |
| 8 | `services.extra-hosts` | `extra_hosts` appears only on `cockpit-probe` and `cockpit-probe-control`, and there it is exactly `host.docker.internal=host-gateway`. That entry gives both probes the same name for the host on every Docker, and it opens no route: the sealed probe sits on an internal network, which has no gateway. |
| 9 | `edge.upstreams` | The edge runs the runner image in `edge` mode without an entrypoint override, and every route of its offset map targets a project service that is not a harness service. The edge also joins `egress`, where Docker DNS forwards unknown names outward, so one route to an outside host would break the seal. |
| 10 | `edge.proxy-pass` | The listen ports of the offset map are exactly the targets of the edge `ports`. |
| 11 | `edge.listen` | The edge listens only on the container ports of the declared offsets (`30000 + offset`). |
| 12 | `edge.host-ip` | Every edge port binds `127.0.0.1`. |
| 13 | `edge.published` | Published ports are the declared offsets inside the leased block, distinct, and as many as the targets. |
| 14 | `mounts.binds` | Bind sources resolve by real path under `build/` for the builder, under `work/` for the runner, under `src/` for project services; the edge, the probes, `cockpit-otlp` and `cockpit-wire` bind nothing. A path that cannot be resolved is a violation. |
| 15 | `mounts.sockets` | No socket is bound. |
| 16 | `volumes.named` | Named volumes are project-owned (`local` driver, no `driver_opts`), and an external volume may serve only the builder. |
| 17 | `privileges.privileged` | `privileged` appears nowhere. |
| 18 | `privileges.devices` | `devices` appears nowhere. |
| 19 | `privileges.namespaces` | `pid`, `ipc` and `userns_mode` appear nowhere. |
| 20 | `privileges.security-opt` | No `security_opt` contains `unconfined`. |
| 21 | `privileges.cap-add` | `cap_add` appears nowhere. |
| 22 | `paths.configs-secrets` | Top-level `configs` and `secrets` read files inside the run directory. |
| 23 | `paths.env-file` | `env_file` appears nowhere. |
| 24 | `paths.build` | Build contexts and additional contexts are inside the run directory, and no build uses `ssh` or `secrets`. |
| 25 | `wire.routes` | In a run that does not record, no service is named or labeled `cockpit-wire`. In a recording run, `cockpit-wire` runs the runner image in `wire` mode for this run id, without an entrypoint override, `volumes` or `tmpfs`, and its routes are exactly `compose.WireRoutes`, each to a project service that is not a harness service. The record then cannot reach a disk, and the recorder can reach no other address. |

Assertions 2 to 24 keep the names and order of the previous cockpit. The edge assertions 9 to 11 read the rendered offset map (the edge `command`) instead of an nginx configuration, and `privileges.cap-add` allows no capability because the edge is the runner binary, not nginx.

## Probes

At network level `open` the hub skips this stage and writes no probe results. At level `sealed`, after the build and before the services start, the hub runs `cockpit-runner probe <canary>` twice: once as `cockpit-probe` on the internal `default` network, then as `cockpit-probe-control` on `egress`. While both run, the hub listens on `<canary host>:<base + 99>` as the canary, where the canary host is `127.0.0.1` on macOS and Windows and the `docker0` address on Linux (see the end of this section). Each run prints one `probe=<n> rc=<code>` line per attempt:

| n | Attempt | Sealed probe expects | Control expects |
| --- | --- | --- | --- |
| 1 | TCP to `1.1.1.1:443` | rc ≠ 0 | rc = 0 |
| 2 | DNS lookup of `example.com` | rc ≠ 0 | rc = 0 |
| 3 | TCP to `host.docker.internal:<canary>` | rc ≠ 0 and the canary accepts no connection | rc = 0 and the canary accepts exactly one |
| 4 | UDP to `1.1.1.1:53` | rc ≠ 0 | rc = 0 |

A sealed attempt that succeeds, or a canary connection during the sealed run, is a violation. A missing result line or a control that does not succeed makes the seal inconclusive, because a sealed failure could then come from the host being offline.

The hub opens the canary through `pipeline.Options.ListenCanary` (default `seal.ListenCanary` on `<base + 99>`). The fake-docker pipeline tests pass a listener on an OS-chosen port and point the fake probe control at it, so no unit test binds a port of a layout block and parallel test processes or a running hub cannot collide with them. The Docker integration tests keep the real canary on their own block.

## Verdict and `seal.json`

The seal verdict over all static and probe results is `failed` when any result is a violation, `error` when none is but one is inconclusive or there are no results, and `passed` otherwise. The hub writes `seal.json` at 600 in the run directory with `checkedAt`, `verdict`, `ok`, and one entry per check (`name`, `ok`, `inconclusive`, `command`, `expected`, `actual`). The command and the actual text pass through the run redactor first.

## Headless path and the unseal check

`cockpit run --project <path> [--suite <name>] [--unseal-check] [--keep] [--out <dir>] [--archive <file>]` runs one commit, the worktree's `HEAD`, without a hub, and closes it with the hub's verdict and the hub's report bundle. A CI job calls it the same way a person does. It reads the declaration, ensures the runner image, picks a free port block from the hub config (skipping `reservedPorts`) and names the run `run-<date>-<time>-<sha7>-h<n>` (`store.HeadlessRunID`). The hub never makes an id that ends in `-h<n>`, so a headless run and a hub run cannot share an id however close together they start. After it ensures the runner image, the run claims the id with the lowest `n` by creating a stopped container `cockpit-claim-<run id>` (label `io.cockpit.claim`, no network) from the runner image. Docker refuses a second container with the same name, so two headless runs that pick the same `n` at once cannot both claim it, whatever temporary directory or user each runs with, and the later one moves to the next `n`. Holding the claim, the run then looks for a container or network that carries the id; a kept run leaves its project after it removes its claim, so when one is found the run removes its claim and moves to the next `n`. The claim commands run on their own 30-second limit, not on the run's context, and removing a claim starts a fresh limit, so an interrupt during the claim cannot leave a claim the run does not know about. The run removes the claim container when it ends, refused, interrupted, failed, passed or kept. In a temporary run directory `cockpit-<run id>-*` it then extracts the commit, generates the environment, writes the run files, runs the static assertions, builds, probes, boots the expected services, runs the suite and writes the report bundle. It removes the project and the run directory at the end unless `--keep` is given.

The verdict comes from the same table and the same rules as a hub run. Each stage ends with the event the hub's pipeline would apply (`pipeline.FailureEvent` classifies a failure), and `lifecycle.Transition` walks the state in memory. So a seal violation closes as `failed` and any other failure before the run stage closes as `error`, both without a report bundle; a run stage that fails or passes its time limit still reports, with the verdict preset; and the report stage builds the bundle with `pipeline.ReportBundle`, the function the hub's report stage calls ([verdict](verdict.md#boundary)). The headless path has no signal stream, no collectors and no wire record, so its `summary.json` marks `signals` as `skipped` and lists no collector stages.

`--out <dir>` copies the report bundle to a new directory with the bundle's modes (directories 700, files 600), and `--archive <file>` writes the [bundle archive](export.md) with the run masker, through `archive.Write` and the same file handling as `cockpit runs export`. Both refuse a path that exists before the run starts, so nothing is replaced, and a run that closes without a bundle writes neither. A copy that fails partway removes the directory it created, so the same `--out` works on the next try.

Every line on the screen passes the run redactor. Each stage prints one line. A failed stage then prints the last 50 lines of its output, and the command ends with the verdict and up to three of its reasons. Child output goes to `run.log` in the run directory. The exit codes are those of the [`cockpit runs` table](api.md#cli) where they overlap:

| Exit | Meaning |
|---|---|
| 0 | verdict `passed` |
| 1 | verdict `failed` |
| 2 | verdict `error`, or the bundle copy, the archive or the cleanup failed |
| 3 | refused before the run: a usage error, an `--out` or `--archive` path that exists, or a failed worktree, declaration, suite, Docker or port block check |
| 5 | verdict `aborted` |
| 130 | interrupted; a run interrupted in its run stage still writes its bundle, `--out` and `--archive` |

`--unseal-check` proves that the probes measure the seal. It starts a second project, `<run>-unsealed`, whose only difference is one extra file setting `networks.default.internal: false`. It runs the same probes there and removes the project. The check passes only when all four sealed attempts turn into violations. A commit at network level `open` has no probes to measure, so `--unseal-check` refuses it in the declaration step (exit 3).

The static assertions are tested against `internal/seal/testdata/config.toy.json`, which the fake Docker of the hub tests also serves as its `compose config`. It is generated, not edited: `make seal-fixture` renders the toy consumer through the real harness and `docker compose config` (run id `r-toy`, run directory `/fixture/runs/r-toy`, port base 22000, runner image `cockpit-runner:0123456789ab`, a fixed `.env`) and writes it with every service `environment` emptied, so no credential value is committed. `TestIntegrationTheCommittedSealFixtureEqualsAFreshRenderOfTheToyConsumer` (with `COCKPIT_INTEGRATION=1`) fails when the committed file differs from a fresh render, for example after the toy's compose files or image inputs change. Compose versions differ in how they print defaults, so the render is normalised before it is compared or written, and only in documented ways: every service `environment` is emptied (credentials), and `bind.create_host_path: true` is dropped from bind volumes, because Compose v2 prints that default explicitly and v5 omits it (the fixture is written with Compose v5.6.1; CI renders it with the Compose of `ubuntu-latest`, v2.38.2 at the time of writing). `TestFixtureNormalisationDropsOnlyTheDefaultCreateHostPathAndKeepsEverySealViolationVisible` runs every violating copy of the static assertion table through the normalisation and requires the same assertion to fail, so the normalisation cannot hide a seal-relevant key. Any other difference fails the drift test: a Compose upgrade that changes the rendering needs a regenerated fixture or a new documented normalisation with that test extended.

`make run` runs `make build`, which builds the embedded runners and then the hub, and runs the headless path on `testdata/toy-project` copied into a fresh git repository. Pass flags with `RUN_ARGS`, for example `make run RUN_ARGS=--unseal-check`.

Both probe services map `host.docker.internal` to `host-gateway`, the address Docker gives for the host. The canary listens where that address leads. On macOS and Windows, where Docker Desktop runs the daemon, it is the host loopback, so the canary listens on `127.0.0.1`. On Linux it is the address of the `docker0` bridge, so the canary listens on the first IPv4 address of `docker0` and never on a wildcard address. `seal.CanaryHost` makes that choice from the hub's operating system. A Linux hub without `docker0` cannot open the canary, and the seal check ends in an error. A daemon whose `host-gateway-ip` points elsewhere gives the control no canary connection, so the seal is inconclusive rather than passed.
