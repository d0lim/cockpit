# Sealing

This page holds the five seals, the static assertion table and the rules of the sealed-network probe. The code is in `internal/seal` (assertions, probes, verdict, `seal.json`), `internal/compose` (harness, compose calls) and `internal/runenv` (the headless path). When this page and the code disagree, this page is fixed first and the code follows. `TestSealingDocumentTableMatchesTheStaticAssertionsBothWays` reads the assertion table below and compares it with the names `seal.Static` returns, in order.

## The five seals

| Seal | What holds it |
| --- | --- |
| Fresh state | Every run gets a new compose project named after the run id, with its own volumes, networks and run directory. Credentials are generated inside the run tree by `env.command` in a networkless one-shot container. Destruction removes the volumes (`down -v`). |
| Isolation between sessions | Each run leases its own port block, its own project name and labels (`io.cockpit.managed`, `io.cockpit.run-id`), and its own run directory opened through `os.Root`. Edge ports bind `127.0.0.1` only. |
| Pinned commit | The source, the suite, the declaration and the consumer overlays come from the git objects of one commit, extracted twice (`src/`, `build/`) and checked against the tree's file count. The harness and the runner come from the cockpit version. |
| Independence from host tools | The hub runs no consumer code on the host: environment generation, building, probing and the suite all run in containers, and every hub git call is hardened against repository configuration. |
| Zero external communication at runtime | The default network and every project network are internal. Among long-running services only the edge joins `egress`, and it proxies only to service names of the project on declared offset ports. The static assertions prove the configuration, and the probes prove the network. |

## Static assertions

The hub renders the run's configuration once with `docker compose ... --profile '*' config --format json` and runs these 24 assertions on it in this order, before any image is built or any service starts. Every assertion runs even after another fails. A run fails the seal when any assertion fails. The results carry only service, network, port and path names, because the rendered configuration holds credential values.

| # | Name | Expectation |
| --- | --- | --- |
| 1 | `roles.present` | Exactly one service holds each harness role (`edge`, `runner`, `probe`, `probe-control`, and `builder` when declared), and every service carries the run labels. Without this, an empty or partial render would pass the rest vacuously. |
| 2 | `files.overlays` | No compose file in the set matches an entry of `compose.forbidden`. |
| 3 | `networks.set` | `default` and `egress` exist, and no network is external. |
| 4 | `networks.internal` | Every network other than `egress` is internal. |
| 5 | `services.networks` | The edge joins every internal network and `egress`; the builder and `probe-control` join only `egress`; every other service joins only internal networks. |
| 6 | `services.ports` | Host ports appear only on the edge. |
| 7 | `services.network-mode` | `network_mode` appears nowhere. |
| 8 | `services.extra-hosts` | `extra_hosts` appears nowhere. |
| 9 | `edge.upstreams` | The edge runs the runner image in `edge` mode without an entrypoint override, and every route of its offset map targets a project service that is not a harness service. The edge also joins `egress`, where Docker DNS forwards unknown names outward, so one route to an outside host would break the seal. |
| 10 | `edge.proxy-pass` | The listen ports of the offset map are exactly the targets of the edge `ports`. |
| 11 | `edge.listen` | The edge listens only on the container ports of the declared offsets (`30000 + offset`). |
| 12 | `edge.host-ip` | Every edge port binds `127.0.0.1`. |
| 13 | `edge.published` | Published ports are the declared offsets inside the leased block, distinct, and as many as the targets. |
| 14 | `mounts.binds` | Bind sources resolve by real path under `build/` for the builder, under `work/` for the runner, under `src/` for project services; the edge and the probes bind nothing. A path that cannot be resolved is a violation. |
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

Assertions 2 to 24 keep the names and order of the previous cockpit. The edge assertions 9 to 11 read the rendered offset map (the edge `command`) instead of an nginx configuration, and `privileges.cap-add` allows no capability because the edge is the runner binary, not nginx.

## Probes

After the build and before the services start, the hub runs `cockpit-runner probe <canary>` twice: once as `probe` on the internal `default` network, then as `probe-control` on `egress`. While both run, the hub listens on `127.0.0.1:<base + 99>` as the canary. Each run prints one `probe=<n> rc=<code>` line per attempt:

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

`cockpit run-env --project <path> [--suite <name>] [--unseal-check] [--keep]` runs one sealed environment of the worktree's `HEAD` without a hub. It reads the declaration, ensures the runner image, picks a free port block from the hub config (skipping `reservedPorts`), extracts the commit, generates the environment, writes the run files, runs the static assertions, builds, probes, boots the expected services and runs the suite. It then removes the project and the run directory unless `--keep` is given. It exits 0 only when every stage passes, 1 when a stage fails, 2 when it refuses to start, and 130 when interrupted. Child output goes to `run-env.log` in the run directory, and every line on the screen passes the run redactor.

`--unseal-check` proves that the probes measure the seal. It starts a second project, `<run>-unsealed`, whose only difference is one extra file setting `networks.default.internal: false`. It runs the same probes there and removes the project. The check passes only when all four sealed attempts turn into violations.

The static assertions are tested against `internal/seal/testdata/config.toy.json`, which the fake Docker of the hub tests also serves as its `compose config`. It is generated, not edited: `make seal-fixture` renders the toy consumer through the real harness and `docker compose config` (run id `r-toy`, run directory `/fixture/runs/r-toy`, port base 22000, runner image `cockpit-runner:0123456789ab`, a fixed `.env`) and writes it with every service `environment` emptied, so no credential value is committed. `TestIntegrationTheCommittedSealFixtureEqualsAFreshRenderOfTheToyConsumer` (with `COCKPIT_INTEGRATION=1`) fails when the committed file differs from a fresh render, for example after the toy's compose files or image inputs change. Compose versions differ in how they print defaults, so the render is normalised before it is compared or written, and only in documented ways: every service `environment` is emptied (credentials), and `bind.create_host_path: true` is dropped from bind volumes, because Compose v2 prints that default explicitly and v5 omits it (the fixture is written with Compose v5.5.1; CI renders it with the Compose of `ubuntu-latest`, v2.38.2 at the time of writing). `TestFixtureNormalisationDropsOnlyTheDefaultCreateHostPathAndKeepsEverySealViolationVisible` runs every violating copy of the static assertion table through the normalisation and requires the same assertion to fail, so the normalisation cannot hide a seal-relevant key. Any other difference fails the drift test: a Compose upgrade that changes the rendering needs a regenerated fixture or a new documented normalisation with that test extended.

`make run-env` runs `make build`, which builds the embedded runners and then the hub, and runs the headless path on `testdata/toy-project` copied into a fresh git repository. Pass flags with `RUN_ENV_ARGS`, for example `make run-env RUN_ENV_ARGS=--unseal-check`.

The canary listens on the host loopback, and probes reach it through `host.docker.internal`. Docker Desktop provides both. A Linux host needs a canary address on the Docker bridge, which comes with Linux host support.
