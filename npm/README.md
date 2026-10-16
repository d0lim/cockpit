# @d0lim/cockpit-sdk

Type declarations for scripts that run under [cockpit](https://github.com/d0lim/cockpit): the `cockpit:flow` module that runner-site flows import, the `cockpit:collect` module that hub-site collectors import, the globals the engine provides, and the tsconfig base they are checked with. The package holds declarations only; the runtime behind them is the `cockpit` binary.

## Install

Pin the same version as the `cockpit` binary that runs your suites:

```sh
bun add -d @d0lim/cockpit-sdk@<version>
```

## Use

Extend the tsconfig base so the declarations are included:

```json
{
  "extends": "@d0lim/cockpit-sdk/tsconfig.base.json",
  "include": ["suites/*.ts"]
}
```

Flows then import everything they need from `cockpit:flow`:

```ts
import { type Ctx, expectStatus, flow, json, poll, stage, step, suite } from "cockpit:flow";
```

`hostApi` in `.cockpit/project.yaml` stays the compatibility key: the runner refuses a commit whose declared host API differs from its own. Without a registry, `cockpit sdk write <dir>` writes the same files from the binary.
