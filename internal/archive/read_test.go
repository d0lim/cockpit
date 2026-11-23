package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var importMutations = []porttest.Mutation{
	{"every entry sits under one run directory", "drop the IsRunID and same-root checks in reader.entry", "red: the archive with two roots unpacks", "TestAnEntryOutsideTheShapeTheHubWritesRefusesTheImport"},
	{"only regular files are taken", "drop the Typeflag check in reader.entry", "red: the symbolic and the hard link unpack as empty files", "TestAnEntryOutsideTheShapeTheHubWritesRefusesTheImport"},
	{"an entry path stays inside its run directory", "drop the Clean and IsLocal checks in reader.entry", "red: the dot-dot entry under source answers an error that is no refusal", "TestAnEntryOutsideTheShapeTheHubWritesRefusesTheImport"},
	{"an entry appears once", "drop the names check in reader.entry", "red: the repeated summary answers an error that is no refusal", "TestAnEntryOutsideTheShapeTheHubWritesRefusesTheImport"},
	{"a report file names a kind", "accept a path without a kind in reader.reportFile and in reader.matches", "red: report/notes.txt unpacks", "TestAnEntryOutsideTheShapeTheHubWritesRefusesTheImport"},
	{"a wire file sits inside a take", "drop the takeOf check in reader.reportFile", "red: the calls under wire/.hidden unpack", "TestAnEntryOutsideTheShapeTheHubWritesRefusesTheImport"},
	{"the index lists exactly the files of the archive", "drop the comparison loops in reader.checkIndex", "red: the unlisted and the resized files unpack", "TestTheIndexMustListExactlyTheFilesOfTheArchive"},
	{"the bundle has its summary and its metadata", "drop the required-file loop in reader.checkBundle", "red: the archive without meta.json fails later with an error that is no refusal", "TestABundleWithoutItsSummaryOrMetadataOrOfAnotherRunIsRefused"},
	{"the metadata reads strictly and names the same run", "decode meta.json with json.Unmarshal and skip the id check in reader.checkBundle", "red: the metadata of another run unpacks", "TestABundleWithoutItsSummaryOrMetadataOrOfAnotherRunIsRefused"},
	{"every file passes the default masking rules", "skip the Mask(text) comparison in reader.reportFile", "red: the planted password in not-proven.md is stored", "TestAFileOrPathTheDefaultRulesStillChangeRefusesTheImportAndIsNamedMasked"},
	{"every report path passes the default masking rules", "skip the Mask(rel) comparison in reader.reportFile", "red: the path with the planted token is stored", "TestAFileOrPathTheDefaultRulesStillChangeRefusesTheImportAndIsNamedMasked"},
	{"a refused path is named masked", "append rel as it is in reader.reportFile", "red: files carries the planted token", "TestAFileOrPathTheDefaultRulesStillChangeRefusesTheImportAndIsNamedMasked"},
	{"each wire call is proven again with the take's public fields", "skip proveTakes in reader.finish", "red: the call with the planted password is stored", "TestAWireCallThatNoLongerProvesCleanRefusesTheImport"},
	{"the report files are stored under report/", "store each file at its path below the run directory in reader.store", "red: the stored files are not under report/", "TestAnArchiveTheHubWroteUnpacksAsItsReportBundleWithItsMetadata"},
	{"a file over the limit is refused before it is read", "drop the size checks in reader.read", "red: the oversized entry is read and answers an error that is no ErrTooLarge", "TestAnEntryOverTheImportLimitIsRefusedBeforeItIsRead"},
}

const importPlanted = "PLANTEDimportsecret93ab"

func TestImportMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "read_test.go", importMutations)
}

func TestAnArchiveTheHubWroteUnpacksAsItsReportBundleWithItsMetadata(t *testing.T) {
	list := exported(t)
	got, stage, err := importInto(t, tarOf(t, list))
	if err != nil {
		t.Fatal(err)
	}
	stored := 0
	for _, e := range list {
		rel, ok := strings.CutPrefix(e.name, runID+"/"+DirReport+"/")
		if !ok {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(stage, DirReport, filepath.FromSlash(rel)))
		if err != nil || string(raw) != e.body {
			t.Errorf("%s: stored %q, %v", rel, raw, err)
		}
		stored++
	}
	for _, gone := range []string{FileIndex, DirSchema} {
		if _, err := os.Lstat(filepath.Join(stage, gone)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is stored", gone)
		}
	}
	if stored < 10 || got.Files != stored || got.RunID != runID || got.Meta.ID != runID || got.Meta.Verdict != contract.VerdictFailed || !got.Meta.Wire || got.Index.RunID != runID {
		t.Fatalf("stored %d, imported %+v", stored, got)
	}
}

func TestAnEntryOutsideTheShapeTheHubWritesRefusesTheImport(t *testing.T) {
	base := exported(t)
	if _, _, err := importInto(t, tarOf(t, base)); err != nil {
		t.Fatalf("the base archive is refused: %v", err)
	}
	other := "run-20261019-010012-abc1234"
	ledger := "report/e2e/ledger.json"
	cases := map[string]struct {
		extra      entry
		path, kind string
	}{
		"two roots":     {entry{name: other + "/" + ledger, body: "{}"}, ledger, "ledger"},
		"no run root":   {entry{name: "notes/" + ledger, body: "{}"}, ledger, "ledger"},
		"symbolic link": {entry{name: runID + "/report/source/link.ts", typ: tar.TypeSymlink, link: "/etc/passwd"}, "report/source/link.ts", "source"},
		"hard link":     {entry{name: runID + "/report/source/link.ts", typ: tar.TypeLink, link: runID + "/report/seal.json"}, "report/source/link.ts", "source"},
		"directory":     {entry{name: runID + "/report/e2e/exec/", typ: tar.TypeDir}, "report/e2e/exec/", "exec"},
		"dot-dot":       {entry{name: runID + "/report/source/../../../escape.ts", body: "x"}, "report/source/../../../escape.ts", "source"},
		"top dot-dot":   {entry{name: runID + "/../escape.json", body: "{}"}, "../escape.json", "source"},
		"absolute":      {entry{name: runID + "//etc/cron.d/x", body: "{}"}, "/etc/cron.d/x", "source"},
		"backslash":     {entry{name: runID + "/report/source/a\\b.ts", body: "x"}, "report/source/a\\b.ts", "source"},
		"repeated":      {entry{name: runID + "/report/summary.json", body: "{}"}, "report/summary.json", "summary"},
		"no kind":       {entry{name: runID + "/report/notes.txt", body: "x"}, "report/notes.txt", "source"},
		"wire, no take": {entry{name: runID + "/report/wire/.hidden/calls.jsonl", body: ""}, "report/wire/.hidden/calls.jsonl", "wire-calls"},
		"other top":     {entry{name: runID + "/run.env", body: "A=1"}, "run.env", "source"},
		"schema shape":  {entry{name: runID + "/schema/x/y.json", body: "{}"}, "schema/x/y.json", "schema"},
	}
	refused := 0
	for name, c := range cases {
		list := indexed(t, append([]entry{c.extra}, base...), listed(c.path, c.kind, c.extra.body))
		_, _, err := importInto(t, tarOf(t, list))
		var invalid *Invalid
		if !errors.As(err, &invalid) || len(invalid.Files) != 1 {
			t.Errorf("%s: want a refusal naming the entry, got %v", name, err)
			continue
		}
		refused++
	}
	if refused != len(cases) || refused != 14 {
		t.Fatalf("refused %d of %d", refused, len(cases))
	}
}

func TestTheIndexMustListExactlyTheFilesOfTheArchive(t *testing.T) {
	base := exported(t)
	extra := entry{name: runID + "/report/e2e/ledger.json", body: `{"flows":{}}`}
	if _, _, err := importInto(t, tarOf(t, indexed(t, append(slices.Clone(base), extra), listed("report/e2e/ledger.json", "ledger", extra.body)))); err != nil {
		t.Fatalf("a listed extra file is refused: %v", err)
	}
	cases := map[string][]entry{
		"unlisted file":  append(slices.Clone(base), extra),
		"listed, absent": indexed(t, base, listed("report/e2e/ledger.json", "ledger", extra.body)),
		"other size": indexed(t, base, func(a *contract.Archive) {
			a.Files[0].Bytes++
		}),
		"other kind": indexed(t, base, func(a *contract.Archive) {
			a.Files[0].Kind = contract.ArchiveKindSeal
		}),
		"other run":  indexed(t, base, func(a *contract.Archive) { a.RunID = "run-20261019-010012-abc1234" }),
		"format two": indexed(t, base, func(a *contract.Archive) { a.Format = 2 }),
		"no index": slices.DeleteFunc(slices.Clone(base), func(e entry) bool {
			return strings.HasSuffix(e.name, "/"+FileIndex)
		}),
	}
	refused := 0
	for name, list := range cases {
		_, _, err := importInto(t, tarOf(t, list))
		var invalid *Invalid
		if !errors.As(err, &invalid) {
			t.Errorf("%s: want a refusal, got %v", name, err)
			continue
		}
		refused++
	}
	if refused != 7 {
		t.Fatalf("refused %d of %d", refused, len(cases))
	}
}

func replaced(list []entry, rel, body string) []entry {
	out := slices.Clone(list)
	for i, e := range out {
		if e.name == runID+"/"+DirReport+"/"+rel {
			out[i].body = body
		}
	}
	return out
}

func TestABundleWithoutItsSummaryOrMetadataOrOfAnotherRunIsRefused(t *testing.T) {
	base := exported(t)
	without := func(rel string) []entry {
		list := slices.DeleteFunc(slices.Clone(base), func(e entry) bool { return e.name == runID+"/"+DirReport+"/"+rel })
		return indexed(t, list, func(a *contract.Archive) {
			a.Files = slices.DeleteFunc(a.Files, func(f contract.ArchiveFile) bool { return f.Path == DirReport+"/"+rel })
		})
	}
	resized := func(rel, body string) []entry {
		return indexed(t, replaced(base, rel, body), func(a *contract.Archive) {
			for i := range a.Files {
				if a.Files[i].Path == DirReport+"/"+rel {
					a.Files[i].Bytes = int64(len(body))
				}
			}
		})
	}
	cases := map[string][]entry{
		"no summary":          without("summary.json"),
		"no meta":             without("meta.json"),
		"meta of another run": resized("meta.json", strings.Replace(entryBody(base, "meta.json"), runID, "run-20261019-010012-abc1234", 1)),
		"meta with a field":   resized("meta.json", strings.Replace(entryBody(base, "meta.json"), "{", `{"slotOf":"x",`, 1)),
		"no final verdict":    resized("meta.json", strings.Replace(entryBody(base, "meta.json"), `"failed"`, `"running"`, 1)),
		"summary of another":  resized("summary.json", strings.Replace(entryBody(base, "summary.json"), runID, "run-20261019-010012-abc1234", 1)),
	}
	refused := 0
	for name, list := range cases {
		_, _, err := importInto(t, tarOf(t, list))
		var invalid *Invalid
		if !errors.As(err, &invalid) {
			t.Errorf("%s: want a refusal, got %v", name, err)
			continue
		}
		refused++
	}
	if refused != 6 {
		t.Fatalf("refused %d of %d", refused, len(cases))
	}
}

func entryBody(list []entry, rel string) string {
	for _, e := range list {
		if e.name == runID+"/"+DirReport+"/"+rel {
			return e.body
		}
	}
	return ""
}

func TestAFileOrPathTheDefaultRulesStillChangeRefusesTheImportAndIsNamedMasked(t *testing.T) {
	base := exported(t)
	result := "# Not proven\n\n- password: " + importPlanted + "\n"
	path := "source/flows/token=" + importPlanted + ".ts"
	list := indexed(t, append(replaced(base, "not-proven.md", result), entry{name: runID + "/report/" + path, body: "x"}), func(a *contract.Archive) {
		for i := range a.Files {
			if a.Files[i].Path == "report/not-proven.md" {
				a.Files[i].Bytes = int64(len(result))
			}
		}
		listed("report/"+path, "source", "x")(a)
	})
	_, stage, err := importInto(t, tarOf(t, list))
	var unmasked *Unmasked
	if !errors.As(err, &unmasked) || !slices.Equal(unmasked.Files[:2], []string{FileIndex, "report/not-proven.md"}) || len(unmasked.Files) != 3 {
		t.Fatalf("want the index, the result and the path refused, got %v", err)
	}
	if strings.Contains(strings.Join(unmasked.Files, " "), importPlanted) || !strings.Contains(unmasked.Files[2], "****") {
		t.Fatalf("the refused path is not masked: %q", unmasked.Files)
	}
	if raw, err := os.ReadFile(filepath.Join(stage, DirReport, "not-proven.md")); err == nil {
		t.Fatalf("the refused file is stored: %s", raw)
	}
}

func TestAnEntryOverTheImportLimitIsRefusedBeforeItIsRead(t *testing.T) {
	var out bytes.Buffer
	zipped := gzip.NewWriter(&out)
	w := tar.NewWriter(zipped)
	w.WriteHeader(&tar.Header{Name: runID + "/report/e2e/result.json", Typeflag: tar.TypeReg, Mode: 0o600, Size: importFileBytes + 1})
	w.Write([]byte("{}"))
	zipped.Flush()
	_, _, err := importInto(t, out.Bytes())
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
	if _, _, err := importInto(t, tarOf(t, exported(t))); err != nil {
		t.Fatalf("an archive under the limit is refused: %v", err)
	}
}

func TestAWireCallThatNoLongerProvesCleanRefusesTheImport(t *testing.T) {
	base := exported(t)
	calls := entryBody(base, "wire/suite/calls.jsonl")
	if !strings.Contains(calls, `"token":"tok-`) {
		t.Fatalf("the take keeps no public token to prove against: %s", calls)
	}
	planted := strings.Replace(calls, `"note":"seeded-call-value"`, `"note":"seeded-call-value","clientSecret":"`+importPlanted+`"`, 1)
	list := withTake(t, base, planted, nil)
	_, stage, err := importInto(t, tarOf(t, list))
	var unmasked *Unmasked
	if !errors.As(err, &unmasked) || !slices.Equal(unmasked.Files, []string{"report/wire/suite/calls.jsonl"}) {
		t.Fatalf("want the calls refused, got %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(stage, DirReport, "wire", "suite", "calls.jsonl")); err == nil {
		t.Fatalf("the refused calls are stored: %d bytes", len(raw))
	}
}
