# Declaration select plan

Goal. One repository keeps more than one declaration under `.cockpit/`, such as one per stack, and each run picks one: `cockpit run --declaration`, `cockpit runs submit --declaration`, the submit body field `declaration` and the action input `declaration`. A declaration can also name its own public env file, so two declarations that share a scripts directory hand their flows different values.

## The problem

The hub and `cockpit run` read `.cockpit/project.yaml` and nothing else (`compose.DeclarationPath` in `ReadDeclaration`, the work tree copy, `envops.Open` and `declaredHostAPI`). A repository that runs one suite on its own stack and another suite on a larger stack had to fold both into one declaration, so every run paid for the larger stack, or keep the second declaration in another repository pinned to an older commit. The runner reads its public env from `scripts/env.json` unless `E2E_ENV` says otherwise, and the hub never sets `E2E_ENV`, so two declarations over one scripts directory could not give their flows different addresses.

## Decisions

- One type carries the choice: `contract.DeclarationFile`. Its zero value is the default, `.cockpit/project.yaml`, and `Path()` returns the file to read. `ParseDeclarationFile` is the only constructor: it maps an empty value and the default path to the zero value and accepts only `.cockpit/<name>.yaml` whose name matches the name pattern (`^[a-z0-9][a-z0-9-]{0,62}$`). That one rule refuses an absolute path, `..`, a path outside `.cockpit/`, a nested path and a name with other characters. Decoding JSON into the type runs the same parse, so a run record or an imported bundle cannot hold another value.
- Every boundary parses: `cockpit run --declaration`, the submit body of `POST /api/runs` (a bad value answers 400 before the scheduler runs, so no run is created), and the import of a bundle archive. `cockpit runs submit` forwards the flag, and the hub's 400 exits 3. The screen's submit form keeps the default and lists the default declaration's suites.
- The selected file is read from the commit like the default: same checks, same rejection kinds, and messages that name the selected file. A commit without it is refused as `missing`.
- Inside the run nothing moves. `MakeWorkTree` copies the selected file to `work/.cockpit/project.yaml`, which the runner sees as `/work/.cockpit/project.yaml`, so the harness, the seal checks and their fixtures are unchanged for every run.
- `store.Meta.Declaration` and `contract.BundleMeta.Declaration` hold the selected file and are omitted for the default, like `wire`. A record or bundle written before this change reads as the default, and the bundle of a default run stays importable by an older hub. `RunItem.declaration` always carries the path, the default included, and `cockpit runs get` prints it.
- The queue merges a submission into a queued run only when the commit, suite, wire option and declaration all match.
- `scripts.env` is an optional declaration field, relative to the repository root like every declared path, that must be a file inside `scripts.dir`. When it is set the harness gives the runner `E2E_ENV=/work/scripts/<path inside scripts.dir>`; when it is not, the runner keeps reading `env.json` at the top of `scripts.dir`, and the harness is byte for byte the same as before.
- The action passes `declaration` as `--declaration` only when it is set, through the variable `COCKPIT_DECLARATION_FILE` (the runner already reads `COCKPIT_DECLARATION`), and the job summary gains a Declaration row.
- The declaration change only adds an optional field, so `hostApi` stays 1. `sdk/` declares no declaration types, so neither `sdk/` nor the host API fixture changes.

## Checklist

- [ ] Docs first: `README.md`, `docs/hub.md` (more than one declaration, the action input), `docs/harness.md` (reading the selected file, the work tree copy, `scripts.env`), `docs/guide.md` (`E2E_ENV` under the hub), `docs/api.md` (submit body, `RunItem.declaration`, CLI flag), `docs/export.md` and `docs/sealing.md` (`cockpit run --declaration`). Evidence: the first commit of this branch.
- [ ] Failing tests first, then code. Evidence: one commit per unit, each with its tests: the declaration file rule and its decode; reading, copying and refusing a selected file; `scripts.env` confinement and `E2E_ENV`; the run record, the queue key, the bundle meta and the import; the CLI, the submit body and `RunItem`; `cockpit run --declaration`; the action input and summary row.
- [ ] Mutations, each red in a scratch copy, recorded at the head of each security control test file.
- [ ] `make verify` on the committed head.
- [ ] Live with the head binary on `testdata/toy-project`: a run without `--declaration` matches trunk; `--declaration .cockpit/alt.yaml` runs the alt suite and reads `scripts/env.alt.json`; `/etc/x.yaml`, `../x.yaml` and `scripts/x.yaml` exit 3 before a run starts; two submissions that differ only in the declaration make two runs; a declaration whose `scripts.env` leaves `scripts.dir` is refused as `confinement`.
- [ ] CI green, including the `action` job with the `declaration` input.

## Left open

- The screen's submit form has no declaration field, so a run of another declaration comes from the CLI, the API or the action.
- `GET /api/suites` lists the default declaration's suites only.
