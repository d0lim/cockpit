# Import from an artifact zip and from the screen

Goal. The file a person gets from a GitHub Actions run opens on the hub without unpacking it first, from the CLI or from the browser.

## The problem

The web downloads an Actions artifact as a zip that holds `cockpit-run.tar.gz`. `cockpit runs import` took only the archive, so the downloaded file was refused with "the body is not gzip", and the screen had no way to import at all.

## Decisions

- The hub unwraps the zip, not the CLI, so the CLI, the API and the screen take the same file (`archive.UnpackFile`). The hub tells the two apart by their first bytes. A zip must hold exactly one regular `*.tar.gz`, stored or deflated, within the body limit, and is read from an unlinked spool, because a zip is read from its end. The archive inside then passes every check of the import.
- `POST /api/runs/import` takes `application/zip` beside `application/gzip`. The CLI picks the type from the file's first bytes.
- The Runs screen gets an `Import` form beside `New run`: one file field, posted as `multipart/form-data` to `POST /runs/import`, which runs the same import (`api.Reader.Import`). On success it answers `HX-Redirect` to the run, so the browser opens it. `HX-Location` was not used, because it fetches into the current target instead of loading the run's page.

## Checklist

- [x] Docs first: `docs/export.md`, `docs/api.md`, `docs/screen.md`, `docs/hub.md`.
- [x] `archive.UnpackFile`. Evidence: `internal/archive/unwrap_test.go`. A deflated and a stored artifact zip unpack like the archive they hold. Seven other zips are refused: two files, none, a directory, another name, another method, a declared size over the limit, and no magic. Refused names are masked, and the stage stays empty.
- [x] API, CLI and screen. Evidence: `TestAnArtifactZipThatHoldsTheArchiveImportsAsTheArchiveDoes`, `TestImportSendsAnArtifactZipAsAZip`, `TestTheRunsScreenImportsAnArtifactZipAndOpensTheRun` and `TestAScreenImportWithoutAnArchiveSaysSoAndAPlainPostIsRedirected`.
- [x] Mutations: 7 new and the 25 of the import, run in a working copy, all red.
- [x] Live: the zip downloaded from the web for a real Actions run imported with `cockpit runs import <zip>` on a check hub. The same zip, picked in the Import form in Aside, opened the run's Overview ("imported from a bundle archive; it ran elsewhere").
- [x] `make verify` in a clean copy. Evidence: vet, `go test -race`, the screen typecheck, the generated-files check, the license gate, the build and the size measurement passed in a fresh copy with its own commit.
- [x] Review round 1 (Greptile on #23). The screen capped the whole form, not the file, so a gzip archive between 128 and 129 MiB passed there while the API refused it. `UnpackFile` now caps every upload at the body limit, the archive as well as the zip (`TestAnUploadOverTheBodyLimitIsRefusedWhetherArchiveOrZip`, its mutation red).
- [ ] CI green on the PR.
