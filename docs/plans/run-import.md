# Run import plan

Goal. A run made away from the hub, such as the bundle archive a GitHub Actions job uploads, opens on the hub's screen with the same pages as a run the hub made: the overview, the report, each flow with its code, and the wire record when there is one.

## The problem

A run made by `cockpit run` leaves its verdict and report bundle as a bundle archive (`docs/export.md`). The archive can be read with the skill or as files, but the screen shows only runs in the hub's own store, and the hub had no way to take an archive in. A team that runs suites in Actions had to open JSON by hand to see what a failed step sent and received.

## Decisions

- `cockpit runs import <file>` posts the archive to `POST /api/runs/import`, behind the same guard as every other write. The hub owns its store, so the archive goes through the hub, not straight into the home directory.
- The hub records an imported run as `done/destroyed` with the new destroy reason `imported`. The pages of a destroyed run already read only the report bundle, so the screen needs no change. Nothing on the hub acts on a destroyed run: it takes no place in the kept pool, holds no lease, accepts no event, and reconciliation leaves it alone.
- The record is built from `report/meta.json` (`store.ImportedMeta`), not copied. A copy would carry `harness`, which makes `RefuseLegacyLayout` refuse the next hub start, and it has no phase or environment. The record takes no slot, no image keys and no repository, because those belong to the host that ran it.
- The archive is untrusted input, and the hub checks all of it before the run appears (`archive.Unpack`). Every entry must be a regular file under one directory named after a run, with a clean, local path. The index must list exactly the files of the archive. A report file must name one of the kinds the archive writes. The bundle metadata must read strictly against its contract. Body and file size limits apply.
- The hub cannot prove the run's credentials are absent, because only the run that made the archive knew them. It passes every file and path through its default masking rules and refuses the archive when they change anything. It proves each wire call again with the take's declared public fields, as the export does, because a plain text pass would mask a public token the take is allowed to keep.
- The run is built in a `.import-*` directory inside `runs/` and moved into place in one rename, under the store lock. A refused or broken import leaves nothing. A run id the hub already holds is a conflict, so importing an archive twice cannot overwrite the first copy or a run the hub made.
- Not done: an upload form on the screen. The CLI covers the Actions case, and a form needs a multipart path and its own guard tests. It can follow.

## Checklist

- [x] Docs first: `docs/export.md` (Importing), `docs/api.md` (route, response, refusals, CLI), `docs/lifecycle.md` (an imported run enters `done/destroyed` directly), `docs/hub.md` and `README.md` (where the import fits). Evidence: the diff of this PR.
- [x] `archive.Unpack` with its checks, in `internal/archive/read.go`, `readcheck.go` and `readwire.go`. Evidence: `internal/archive/read_test.go`. An archive written by `archive.Write` unpacks into exactly its report files, and 14 malformed entry shapes, 7 index mismatches and 6 bundle faults are each refused. A planted password, a planted token in a path, a planted secret in a wire call and an oversized entry are refused, and the refused names are masked.
- [x] `store.ImportStage`, `store.Adopt` and `store.ImportedMeta`. Evidence: `internal/hub/store/import_test.go`. The adopted run lists as destroyed by import, without slot, image keys or repository, and the hub still starts. A second import of the same id and a stage outside `runs/` are refused.
- [x] `POST /api/runs/import` and `cockpit runs import`. Evidence: `internal/hub/api/import_test.go` and `internal/hub/cli/import_test.go`. An imported archive answers 201 and opens as a destroyed run with a report, and the second import is a conflict. A request without the guard header, of another media type, not gzip, or carrying a planted password is refused and leaves nothing in `runs/`.
- [x] Mutations, each run in a scratch copy, each red. They are recorded at the head of `read_test.go` and `import_test.go`.
- [x] A live import of the archive from a real GitHub Actions run of a consumer suite (31 report files, 133 steps, `cockpit run` 0.6.0) into a check hub. Evidence: the CLI printed the run id, `passed` and the run page. Its overview, report, flow and environment pages answered 200, a flow page showed its code from the bundle, the signals page said the bundle holds no frozen signals, the export answered 200, and the second import exited 5 with `CONFLICT_STATE`.
- [x] `make verify` in a clean copy. Evidence: vet, `go test -race`, the screen typecheck, the generated-files check, the license gate, the build and the size measurement all passed in a fresh copy with its own commit.
- [x] Review round 1 (Greptile on #21, four findings, each checked against the code and fixed with a test and a recorded mutation):
  - A wire head could name any length, and `wire.Store.CallAt` allocated it. Each head must now point at exactly one line of its calls, and `CallAt` refuses a head outside the take limit (`TestAWireIndexWhoseHeadsMissTheLinesOfItsCallsIsRefused`, `TestCallAtRefusesAHeadOutsideTheTakeLimitBeforeItAllocates`).
  - A JSON escape such as `\u0070assword` hid a key from the text pass. JSON files are now checked again once decoded (`TestAnEscapedSecretInAJSONFileIsFoundOnceItIsDecoded`).
  - Empty files escaped the byte limits. An archive now holds at most 10,000 entries (`TestAnArchiveWithMoreEntriesThanTheLimitIsRefused`).
  - Reading stopped at the tar end, before the gzip footer. The stream is now read to its end, so a cut or corrupt stream is refused, and at most 1 MiB may follow the tar (`TestAGzipStreamCutShortCorruptOrWithDataAfterTheTarIsRefused`).
  The real archive from the Actions run still imports and opens after these checks.
- [ ] CI green on the PR.
