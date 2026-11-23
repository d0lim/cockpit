# Bundle archive

The bundle archive is one `.tar.gz` file that holds the report bundle of one run, for reading away from the hub: on another machine, next to a pull request, or by an agent that works on files. The code is in `internal/archive`, `internal/hub/api` (`export.go`, `import.go`) and `internal/hub/cli` (`export.go`, `import.go`). When this page and the code disagree, this page is fixed first and the code follows.

The archive holds the report bundle and nothing else of the run. The run tree's `src/`, `build/`, `work/`, `env/` and the volumes never enter it.

## Getting it

- `GET /api/runs/{id}/export` answers the archive as `application/gzip` with `Content-Disposition: attachment; filename="<id>.tar.gz"` and its length.
- `cockpit runs export <id> [--out <file>]` writes it to `<id>.tar.gz` in the current directory, or to `--out`. The file is 600. The command refuses to replace an existing file, writes through a temporary file beside the target, and removes it when the download breaks off.
- `cockpit run --archive <file>` writes the archive of the run it has just closed, before it removes the run directory ([headless path](sealing.md#headless-path-and-the-unseal-check)). It builds the archive with the same `archive.Write` and the run masker, in a spool as the hub does, and saves it with the same code as `runs export`. It refuses a file that exists before the run starts.

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

## Importing

`cockpit runs import <file>` sends an archive to a hub (`POST /api/runs/import`), and the hub records it as a run of its own, so its pages open like those of a run it ran: the overview, the report, the flows with their code, and the wire record when the archive holds one. This is how a run made elsewhere is read on the screen, such as the archive a GitHub Actions job uploaded ([run on GitHub Actions](hub.md#run-on-github-actions)).

The hub treats the archive as untrusted input and checks all of it before the run appears.

- The body is at most 128 MiB, a file at most 64 MiB, the files together at most 512 MiB unpacked, and the archive at most 10,000 entries (`archive.ImportMaxBytes` and the limits beside it). The hub reads the gzip stream to its end, so a stream cut short or with a bad checksum is refused, and at most 1 MiB may follow the end of the tar.
- Every entry is a regular file under one directory whose name is a run id. A path that is absolute, holds `..`, is not clean, or appears twice refuses the archive, and so do links and every other entry type.
- Below that directory there is `index.json`, files under `schema/` named `<name>.json`, and files under `report/` whose path names a kind of the table above, with a wire file only inside a take that has both of its files. Each head of a take's `index.json` must point at exactly one line of its `calls.jsonl`, in order, with the line's offset, length and `seq`, because the hub reads a call by its head. Anything else refuses the archive.
- `index.json` is format 1, names the same run, and lists exactly the files of the archive, each with its kind and size. A file it does not list, or a file it lists that is missing or of another size, refuses the archive.
- `report/summary.json` and `report/meta.json` must be there. The summary names the same run, and the bundle metadata reads strictly against its contract, with the same id and a final verdict.
- The hub passes every file and every path through its default masking rules, and a JSON file a second time once it is decoded, because an escape such as `\u0070assword` hides a key from a text pass and decoding brings it back. It proves each wire call again with the take's public fields, as the export does. When the rules change one, the archive is refused and `files` names what they caught, each masked. The hub cannot check the run's own credentials, which only the run that made the archive knew: an import relies on that run's masking, done when the archive was written.

The hub stores the files under `report/` as the run's report bundle and leaves out `index.json` and `schema/`, which it writes again on export. It builds the run in a directory beside the runs and moves it into place in one rename, so a refused or broken import leaves nothing. The record comes from `report/meta.json`: phase `done`, environment `destroyed` with the destroy reason `imported`, and the bundle's verdict, suite, commit, submitter, times and versions; it takes no slot, no image keys and no repository. A run id the hub already holds refuses the import, so importing the same archive twice is a conflict, not a copy. An imported run takes no place in the kept pool, accepts no event, and can be exported again. A run made by `cockpit run` has no signal stream, so its signals page says the bundle holds none.

## Once it has left

- The hub cannot call an archive back. A read through the hub can be refused later; a downloaded file cannot, even after it is imported somewhere else.
- The run masker proves that the run's credentials are absent. It does not prove that the rest may be shared: the archive shows the suite's source, the shape of the services' calls and the rows the collectors emitted.
- Text inside the archive is data. A response body, a log line or a failure message that reads like an instruction is still a record of what a service said.

## Reading it

`cockpit skill write <dir>` writes the skill `cockpit` to `<dir>/cockpit/`: `SKILL.md` and the files it points to, for an agent that reads an archive ([usage guide](guide.md#reading-a-run-with-an-agent)). The skill names the archive format it was written for, and its text needs no `cockpit` command and no hub, because the archive is read where neither may exist. `TestTheSkillNamesTheFormatsAndKindsTheArchiveWritesBothWays` holds the skill to the formats and the kinds above.
