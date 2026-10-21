# @d0lim/cockpit-sdk

Type declarations for scripts that run under [cockpit](https://github.com/d0lim/cockpit): the `cockpit:flow` module that flows import, the `cockpit:collect` module for hub collectors, the globals the runner's engine provides, and the tsconfig base they are checked with. The declarations carry doc comments, so editors and agents see what each port does. The package holds no runtime code; the `cockpit` binary provides the implementation.

`GUIDE.md` in this package is the usage guide for suites and the runner, and `HUB.md` takes a project to its first sealed run on the hub. `declaration.schema.json` is the JSON Schema of `.cockpit/project.yaml`, with a description on every field; point a YAML editor at `node_modules/@d0lim/cockpit-sdk/declaration.schema.json`.

## Install

Pin the same version as the `cockpit` binary that runs your suites:

```sh
bun add -d @d0lim/cockpit-sdk@<version>
```

## Use

`cockpit init` writes a working layout, including this tsconfig in `e2e/`:

```json
{
  "extends": "@d0lim/cockpit-sdk/tsconfig.base.json",
  "include": ["**/*.ts"]
}
```

A project with its own tsconfig can load the declarations through `types` instead:

```json
{ "compilerOptions": { "types": ["@d0lim/cockpit-sdk"] } }
```

Flows import everything from `cockpit:flow`:

```ts
import { type Ctx, expectStatus, flow, json, poll, stage, step, suite } from "cockpit:flow";
```

The declarations include the engine globals (`Buffer`, `URL`, `console` and a few more). They can clash with `@types/node`, so keep the flows in a tsconfig of their own; flows cannot use Node APIs anyway.
