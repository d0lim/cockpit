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

Start from any directory that has a `compose.yaml`. This example uses a single nginx service.

```yaml
services:
  web:
    image: nginx:alpine
```

Add the project declaration, the public env, an empty private env and a suite:

```sh
mkdir -p .cockpit e2e/suites .e2e
printf 'hostApi: 1\n' > .cockpit/project.yaml
printf '{ "baseUrl": "http://web" }\n' > e2e/env.json
printf '{ "local": {} }\n' > .e2e/e2e.private.env.json
```

`e2e/suites/smoke.ts`:

```ts
import { expectStatus, flow, stage, step, suite } from "cockpit:flow";

const home = flow({
	id: "home",
	title: "Home page",
	stage: "basic",
	once: false,
	requires: [],
	steps: [
		step({
			id: "get",
			title: "GET / answers 200",
			kind: "request",
			async run(ctx) {
				const res = await ctx.http.send("GET", `${ctx.env.baseUrl}/`);
				expectStatus(res, 200);
			},
		}),
	],
});

export default suite({ name: "smoke", stages: [stage({ id: "basic", title: "Basic" })], flows: [home] });
```

Attach the runner with a compose overlay, `compose.cockpit.yaml`:

```yaml
services:
  runner:
    image: ${COCKPIT_RUNNER_IMAGE:?run cockpit runner-image first}
    pull_policy: never
    profiles: ["runner"]
    volumes:
      - ./e2e:/work/scripts:ro
      - ./.cockpit/project.yaml:/work/.cockpit/project.yaml:ro
      - ./.e2e:/work/e2e
```

Run it:

```sh
export COCKPIT_RUNNER_IMAGE=$(cockpit runner-image)
docker compose -f compose.yaml -f compose.cockpit.yaml up -d web
docker compose -f compose.yaml -f compose.cockpit.yaml run --rm runner run smoke
```

```
home › get passed 3ms [200]
```

The exit code is 0 when every step passed. `.e2e/` now holds `manifest.json`, `result.json`, `ledger.json` and `context.json`. Add `.e2e/` to `.gitignore`.

The [usage guide](docs/guide.md) covers the rest: the mounts and environment, writing flows, the `ctx` ports, running one flow, the output files, exit codes and type checking.

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

A `v*` tag publishes the binaries to GitHub Releases, the cask to [d0lim/homebrew-tap](https://github.com/d0lim/homebrew-tap) and the `@d0lim/cockpit-sdk` package to npm, all with the tag's version. `hostApi` in `.cockpit/project.yaml` is the compatibility key: the runner refuses a declaration whose host API differs from its own.

## Development

```sh
mise install
make verify
```

`make verify` runs vet, `go test -race`, the generated-files diff, the license gate, the consumer-name check, the no-comment check, the build and the binary size measurement. The rules are in `AGENTS.md`, and the text of the run rules is in `docs/`.

## License

MIT. The full text is in `LICENSE`.
