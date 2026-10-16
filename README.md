# cockpit

cockpit is a run hub that takes one commit of a service that starts with compose, runs it E2E in a sealed environment, and produces the verdict, observations and report bundle in the same shape. A single hub binary (`cockpit`) owns the sealed compose environment and the run state, and the runner embedded in it (`cockpit-runner`) runs TypeScript flows inside the sealed network. Consumer repositories bring only flows, collector scripts, compose overlays and a project declaration.

## Status

The runner host is under construction. The hub commands are `sdk write <dir>` (writes the type declarations and the tsconfig base into the consumer repository), `runner-image` (ensures the `cockpit-runner:<hash>` image from the embedded runner and prints its tag) and `--version`. The runner commands are `run <suite> [--flow <id>]...`, `probe <canary-port>` and `edge <port>=<host>:<port>...`. The `cockpit:flow` module exports `suite`, `stage`, `flow` and `step` plus the helpers `target`, `expectStatus`, `json` and `poll`, so flows need no helper code of their own.

## SDK types

Flows and collectors are type-checked against declarations that cockpit generates. A consumer repository takes them from npm and pins the same version as the `cockpit` binary it runs:

```sh
bun add -d @d0lim/cockpit-sdk@<version>
```

```json
{ "extends": "@d0lim/cockpit-sdk/tsconfig.base.json", "include": ["suites/*.ts"] }
```

`hostApi` in `.cockpit/project.yaml` stays the compatibility key. Without a registry, `cockpit sdk write <dir>` writes the same files from the binary. The release workflow publishes the package from the same tag as the binaries.

## Development

```sh
mise install
make verify
```

`make verify` runs vet, `go test -race`, the generated-files diff, the license gate, the consumer-name check that requires 0 matches, the build and the binary size measurement in one invocation. The consumer-name check runs only when the environment variable `COCKPIT_CONSUMER_NAMES` holds a regular expression. The rules are in `AGENTS.md`, and the text of the run rules is in `docs/`.

## License

The license is MIT. The full text is in `LICENSE`.
