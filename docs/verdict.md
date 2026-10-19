# Verdict

This page holds the 15 verdict rules, the report bundle and the boundary the run pipeline calls. The code is in `internal/report`. When this page and the code disagree, this page is fixed first and the code follows. `TestVerdictDocumentTableMatchesTheRulesBothWays` reads the rule table below and compares it with the rule list in `internal/report/rules.go`, row by row and in order.

## Rules

A run does not pass on its exit code alone. The hub reads the rules from the top, and the first rule that holds sets the verdict. A run is `passed` only when no rule holds.

| Rule | Condition | Verdict |
|---|---|---|
| 1 | A seal violation exists, or the seal comparison could not be made | failed, error |
| 2 | The expected services did not all come up | error |
| 3 | The runner was killed for lack of memory | error |
| 4 | There is no runner exit code (time limit exceeded or aborted) | error |
| 5 | The runner exit code is not 0, 1 or 2 | error |
| 6 | The manifest is missing or could not be read | error |
| 7 | The result is missing, has `complete: false`, or does not match the schema | error |
| 8 | The step ids of the result differ from the manifest or repeat | error |
| 9 | There are no flows or no steps | error |
| 10 | A finished result still has steps that did not run | error |
| 11 | A `request` step passed but sent no request (a vacuous step) | error |
| 12 | The exit code is 0 but there are failed or skipped steps | error |
| 13 | A step failed | failed |
| 14 | A step was skipped without a failed ancestor (a wrong prerequisite) | error |
| 15 | The exit code is 2, or the exit code is 1 with no failed or skipped steps | error |

- Rule 2 also holds when there is no boot verdict at all, because the run closed before booting.
- Rule 11 reads `requests` from `result.json`. The runner's Go http port writes that count (`internal/runner/driver.go`), not the suite script, so the hub needs no second count.
- The first reason names the rule's condition with flow and step ids only. When the result has a stop point, the second reason is `flow › step — message`, where the message is the runner's own masked text. A result that did not finish stops at its last `running` step.
- A verdict the lifecycle set before the report (for example `aborted`) wins. The rule verdict is then kept as the first reason.
- Skipped steps and the stages a suite declares as not proven do not block the verdict. They are not counted as passed either and appear under `notProven`.
- Collector failures and dropped signals never change the verdict. They appear only under `notProven.unread`.

## Report bundle

`report.Assemble` writes the bundle to `runs/<id>/report.partial/` and renames it to `runs/<id>/report/` only when the rescan is clean. Directories are 700 and files 600.

```text
report/
  summary.json             verdict, reasons, step counts, flow table, stop point, not proven, boot, seal, runner,
                           collector stages, reproduction values and versions (format 3)
  meta.json                chosen fields of the run meta (contract.BundleMeta)
  not-proven.md            three sections: skipped steps, not proven as declared by the suite, unread records
  e2e/                     the runner files: manifest, result, ledger, context and exec/*.json
  source/                  the suite source files the manifest points to, and source/source-index.json
  signals/stream.jsonl     the frozen signal segment
  <caller files>           anything else the caller passes, such as seal.json and collector outputs
```

Every file is masked with the run masker as it is written. After writing, `mask.Rescan` reads the whole directory again with the same masker. If any value is still unmasked, the hub:

1. removes `report.partial/` and creates no `report/`,
2. sets `redaction: {status: failed, files: [...]}` in the run meta, with the paths inside the bundle only,
3. calls `RefuseReads` on the run's signal stream, so its reads stop for good.

The usual cause is a signal segment frozen with an older value list than the one the report uses.

`summary.json` is `format: 3`. Compared with format 2 it drops one consumer-specific status key and adds `cockpitVersion`, `hostApi` and `runnerImage`. Its `stages` keys are collector ids plus the hub's own stages (`runner.<file>`, `source`, `signals`). Each stage is `ok`, `skipped`, `absent` or `error`.

## Boundary

The run pipeline calls the package at its report stage with plain inputs. The package does not start containers, call Docker or read the run store.

| Call | Input | Output |
|---|---|---|
| `ReadRunnerFiles(dir)` | the directory the runner wrote | `Collected{Manifest, Result, Files, Stages}`. Each file is read with `os.Root`, no symlinks, at most 4 MiB. The read status is `missing`, `unsafe`, `parse` or `schema` |
| `SuiteSources(suiteDir, manifest)` | the extracted suite directory | files under `source/` and the stage |
| `CollectorUnread(id, outcome)` | one collector's `worker.Outcome` | its stage and its unread records, grouped with counts |
| `Judge(Inputs)` | `Inputs{Manifest, Result, Boot, Seal, RunnerCode, OOMKilled}` | `Judgement{Verdict, Rule, Reasons, Skipped, Declared}` |
| `Assemble(Bundle)` | `Bundle{RunDir, Meta, Inputs, Summary, Files, Unread, Mask, Signals, Patch}` | `Outcome{Judgement, Dir, Redaction, Frozen}` |

- `Bundle.Files` holds paths inside the bundle. `summary.json`, `meta.json`, `not-proven.md` and `signals/` are reserved, and a path outside the bundle is refused before anything is written.
- `Bundle.Signals` is the run's `*signal.Stream`. `Assemble` calls `Freeze` into the bundle first, so the dropped counts reach `notProven.unread`. A failed freeze is an unread record, not an error. A second assembly of the same run finds the stream already frozen and records that too.
- `Bundle.Patch` changes the run meta, for example `func(f func(*store.Meta)) error { _, err := st.PatchMeta(id, f); return err }`. It is called only when the rescan fails.
- `Bundle.Summary` carries the values the summary copies: run id, suite, commit, `harness`, `hubCommit`, host API, cockpit version, runner image, ports, image keys and digests, versions, collector stages, runner outcome and the time. The hub fills `harness` with `compose.HarnessVersion` and `hubCommit` with the VCS revision of the hub binary; a build without VCS information records its version there instead.
- `Outcome.Judgement` is masked with the run masker, so its reasons can go to the run meta or a log as they are. `Outcome.Dir` is empty when the bundle was discarded. `Outcome.Redaction` is then set, and the error, if any, comes from `Patch` or `RefuseReads`.
