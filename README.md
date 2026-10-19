# cockpit

cockpit runs end-to-end flows against a compose stack. You write flows in TypeScript, and the cockpit runner executes them in a container on the stack's network, then writes the results as JSON. One `cockpit` binary carries the runner, so the version you install is the version that runs your flows.

The sealed-run hub (one commit, a fresh sealed environment, a verdict and a report bundle) is under construction. Today the binary builds the runner image and writes the type declarations; the runner executes suites against a stack you start yourself.

## Install

```sh
brew install --cask d0lim/tap/cockpit
```

or with [mise](https://mise.jdx.dev), globally or pinned per project:

```sh
mise use -g github:d0lim/cockpit
mise use github:d0lim/cockpit@0.1.0
```

Docker is required: `cockpit runner-image` imports the embedded runner as a local image.

## Quick start

Start from a directory with a compose project. This example has one nginx service named `web`:

```yaml
services:
  web:
    image: nginx:alpine
```

Scaffold the suite and run it:

```sh
cockpit init --base-url http://web
export COCKPIT_RUNNER_IMAGE=$(cockpit runner-image)
docker compose -f compose.yaml -f compose.cockpit.yaml up -d web
docker compose -f compose.yaml -f compose.cockpit.yaml run --rm runner run smoke
```

```
home › get passed 3ms [200]
missing › get passed 1ms [404]
```

The exit code is 0 when every step passed. `cockpit init` wrote:

| Path | Holds |
| --- | --- |
| `.cockpit/project.yaml` | the project declaration (`hostApi: 1`) |
| `e2e/env.json` | public values for flows; every origin a flow calls must be one of them |
| `e2e/suites/smoke.ts` | a two-flow example suite |
| `e2e/tsconfig.json` | type checking against `@d0lim/cockpit-sdk` |
| `compose.cockpit.yaml` | the runner service and its mounts |
| `.e2e/` | run state and the private env, ignored by git |

`init` refuses to overwrite existing files. The [usage guide](docs/guide.md) covers the rest: the mounts and environment, writing flows, the `ctx` ports, running one flow, the output files, exit codes and type checking.

## Type checking

Flows are checked against declarations that cockpit generates. Take them from npm and pin the same version as the binary:

```sh
bun add -d @d0lim/cockpit-sdk@<version>
```

```json
{ "extends": "@d0lim/cockpit-sdk/tsconfig.base.json", "include": ["suites/*.ts"] }
```

Without a registry, `cockpit sdk write <dir>` writes the same files from the binary.

## Releases

A `v*` tag builds the binaries with GoReleaser and attaches them to the GitHub release; mise installs from there. The same tag publishes `@d0lim/cockpit-sdk` to npm through trusted publishing, so the workflow needs no stored token. The cask in [d0lim/homebrew-tap](https://github.com/d0lim/homebrew-tap) is written by that repository's own workflow from the published release. `hostApi` in `.cockpit/project.yaml` is the compatibility key: the runner refuses a declaration whose host API differs from its own.

## Development

```sh
mise install
make verify
```

`make verify` runs vet, `go test -race`, the generated-files diff, the license gate, the no-comment check, the build and the binary size measurement. The rules are in `AGENTS.md`, and the text of the run rules is in `docs/`.

## License

MIT. The full text is in `LICENSE`.
