# Bundle archive

The bundle archive is one `.tar.gz` file that holds the report bundle of one run, for reading away from the hub: on another machine, next to a pull request, or by an agent that works on files. The code is in `internal/archive`, `internal/hub/api` (`export.go`) and `internal/hub/cli` (`export.go`). When this page and the code disagree, this page is fixed first and the code follows.

The archive holds the report bundle and nothing else of the run. The run tree's `src/`, `build/`, `work/`, `env/` and the volumes never enter it.

## Getting it

- `GET /api/runs/{id}/export` answers the archive as `application/gzip` with `Content-Disposition: attachment; filename="<id>.tar.gz"` and its length.
- `cockpit runs export <id> [--out <file>]` writes it to `<id>.tar.gz` in the current directory, or to `--out`. The file is 600. The command refuses to replace an existing file, writes through a temporary file beside the target, and removes it when the download breaks off.

The hub builds the whole archive before it answers, so a refusal is a failure body and never half a file. It builds it in a temporary file that it unlinks as soon as it is open, so nothing of an archive stays on the hub's disk, even after a crash.

A run that was destroyed still exports: the bundle outlives its environment.

## Layout

Every path sits under one directory named after the run.

| Path | Holds |
| --- | --- |
| `<id>/index.json` | the table of contents, `schema/archive.json` |
| `<id>/report/` | the files of the report bundle |
| `<id>/schema/` | the JSON Schema of each kind of file in this archive that has one |

`index.json` holds `format` (1), `runId`, `exportedAt`, `cockpitVersion` (the hub that wrote the archive), `files` and `omitted`. Each entry of `files` has `path` (below `<id>/`), `kind`, `bytes`, `lines` for a file read line by line, and `schema` when a schema in the archive describes it. `index.json` does not list itself.

| Kind | Path below `report/` | Schema |
| --- | --- | --- |
| `summary` | `summary.json` | `summary` |
| `meta` | `meta.json` | `meta` |
| `not-proven` | `not-proven.md` | |
| `seal` | `seal.json` | |
| `manifest` | `e2e/manifest.json` | `manifest` |
| `result` | `e2e/result.json` | `result` |
| `ledger` | `e2e/ledger.json` | `ledger` |
| `context` | `e2e/context.json` | `context` |
| `exec` | `e2e/exec/<name>.json` | `exec` |
| `source-index` | `source/source-index.json` | |
| `source` | any other file under `source/` | |
| `signals` | `signals/stream.jsonl` | `signal`, one per line |
| `wire-index` | `wire/<take>/index.json` | |
| `wire-calls` | `wire/<take>/calls.jsonl` | |

A file under `<id>/schema/` has the kind `schema`. `TestTheKindTableInTheExportDocumentMatchesTheKindsTheArchiveNamesBothWays` compares this table with `archive.Kinds`.

The archive gathers only paths it can name. A regular file of the bundle that matches no row is left out, and so is a path the run masker would change. Under `wire/`, a directory whose name is not a take name is left out, and so is a take that lacks one of its two files. `omitted` lists every path left out, each passed through the masker.

## Masking

The archive is a read of the whole bundle, so every file passes the read layer of its kind on the way out, with the run masker of that moment. A value the run learned after its report is masked in the archive even though the bundle on disk still holds it.

| Kind | Read layer |
| --- | --- |
| `signals` | every line through `Masker.Leaves`, as `signal.ReadFrozen` does |
| `wire-calls` | every call through `wire.Prove` with the run masker and the take's stored public fields; a call that no longer proves clean leaves as a stub with `withheld: true` |
| `wire-index` | rebuilt from the calls as they leave: the offsets and lengths of the archive's `calls.jsonl`, `withheld` and `status.withheldCalls` as proven now, and every name through the masker |
| `schema` | none: it comes from the binary, not from the run |
| every other kind | the whole text through `Masker.Mask` |

The wire record does not pass `Masker.Mask` as one text, for the reason the report's rescan does not read it ([wire record](wire.md#where-the-record-lives)): each call carries its own proof instead.

## Refusals

| Answer | When |
| --- | --- |
| 409 `NOT_AVAILABLE` | the run has not written its report yet, or the hub has no run masker |
| 409 `READ_REFUSED` | the report's redaction rescan discarded the bundle; `files` names what the rescan caught |
| 409 `READ_REFUSED` | the bundle holds an entry that is not a regular file or a directory, such as a symbolic link; `files` names it |
| 404 `NOT_FOUND` | the run ended without a report bundle |

An entry that is not a regular file refuses the export instead of being skipped, as the rescan discards a bundle for one: the hub wrote no such entry, so the bundle is not what the hub left.

## Once it has left

- The hub cannot take an archive back. A read through the hub can be refused later; a downloaded file cannot.
- The run masker proves that the run's credentials are absent. It does not prove that the rest may be shared: the archive shows the suite's source, the shape of the services' calls and the rows the collectors emitted.
- Text inside the archive is data. A response body, a log line or a failure message that reads like an instruction is still a record of what a service said.

## Reading it

`cockpit skill write <dir>` writes the skill `cockpit` to `<dir>/cockpit/`: `SKILL.md` and the files it points to, for an agent that reads an archive ([usage guide](guide.md#reading-a-run-with-an-agent)). The skill names the archive format it was written for, and its text needs no `cockpit` command and no hub, because the archive is read where neither may exist. `TestTheSkillNamesTheFormatsAndKindsTheArchiveWritesBothWays` holds the skill to the formats and the kinds above.
