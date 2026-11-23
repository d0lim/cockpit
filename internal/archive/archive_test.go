package archive

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/wire"
)

var archiveMutations = []porttest.Mutation{
	{"every text file passes Masker.Mask on the way out", "write the bytes read from the bundle in writer.text", "red: the planted value is in eleven files", "TestEveryFileOfTheBundleLeavesThroughTheRunMaskerWithThePlantedValueHidden"},
	{"the frozen signals pass the masker line by line", "copy the stream file into the spool in writer.signals", "red: the planted value is in signals/stream.jsonl", "TestEveryFileOfTheBundleLeavesThroughTheRunMaskerWithThePlantedValueHidden"},
	{"each wire call is proven again with the run masker", "write the stored call in wire.Export", "red: the planted value is in wire/suite/calls.jsonl", "TestEveryFileOfTheBundleLeavesThroughTheRunMaskerWithThePlantedValueHidden"},
	{"the wire index leaves with masked names", "keep the stored head in wire.Export", "red: the planted value is in wire/suite/index.json", "TestEveryFileOfTheBundleLeavesThroughTheRunMaskerWithThePlantedValueHidden"},
	{"an entry that is not a regular file refuses the archive", "drop the odd entries in scan", "red: the archive is written", "TestAnEntryThatIsNotARegularFileRefusesTheArchive"},
	{"a file no kind names is left out", "write a file without a kind as text in writer.bundle", "red: notes.txt is in the archive", "TestAFileNoKindNamesAndAPathTheMaskerChangesAreLeftOutAndListedMasked"},
	{"a path the masker changes is left out", "drop the Mask(rel) comparison in writer.bundle", "red: an entry name carries the planted value", "TestAFileNoKindNamesAndAPathTheMaskerChangesAreLeftOutAndListedMasked"},
	{"a left-out path is listed masked", "list the path as it is in writer.omit", "red: omitted carries the planted value", "TestAFileNoKindNamesAndAPathTheMaskerChangesAreLeftOutAndListedMasked"},
	{"a take directory with a name that is no take is left out", "accept every directory under wire in takeOf", "red: the partial take is in the archive", "TestAFileNoKindNamesAndAPathTheMaskerChangesAreLeftOutAndListedMasked"},
	{"a take without both of its files is left out", "export a take whatever its file count in writer.bundle", "red: the archive fails on the missing index", "TestAFileNoKindNamesAndAPathTheMaskerChangesAreLeftOutAndListedMasked"},
	{"the index is rebuilt from the calls as they leave", "keep the stored offset and length in wire.Export", "red: a head points into the middle of a line", "TestTheWireIndexInTheArchivePointsAtTheCallsAsTheyLeave"},
	{"only the schemas of the kinds present are added", "add every embedded schema in writer.finish", "red: schema/declaration.json is in the archive", "TestTheArchiveHoldsTheSchemaOfEachKindPresentAndOfItsOwnIndex"},
	{"a bundle without its summary is not exported", "drop the summary check in Write", "red: a directory with only a take is exported", "TestARunWithoutASummaryIsNotAnArchive"},
	{"the spool leaves no file on disk", "skip os.Remove in OpenSpool", "red: the temporary directory holds the spool", "TestTheSpoolIsUnlinkedAsSoonAsItIsOpen"},
}

func TestArchiveMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "archive_test.go", archiveMutations)
}

func TestEveryFileOfTheBundleLeavesThroughTheRunMaskerWithThePlantedValueHidden(t *testing.T) {
	runDir := bundle(t)
	plain, err := export(t, runDir, masker(t))
	if err != nil {
		t.Fatal(err)
	}
	u, err := export(t, runDir, masker(t, planted))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, f := range u.index.Files {
		if f.Kind == contract.ArchiveKindSchema {
			continue
		}
		before, after := plain.files[runID+"/"+f.Path], u.files[runID+"/"+f.Path]
		if !strings.Contains(before, planted) {
			t.Fatalf("%s: the fixture plants nothing", f.Path)
		}
		if strings.Contains(after, planted) || !strings.Contains(after, marker) {
			t.Errorf("%s: want the planted value gone and %s present in %s", f.Path, marker, after)
		}
		if int64(len(after)) != f.Bytes {
			t.Errorf("%s: %d bytes listed, %d written", f.Path, f.Bytes, len(after))
		}
		checked++
	}
	if checked != 14 || len(u.files) != 24 || strings.Contains(u.all, planted) {
		t.Fatalf("checked %d bundle files of %d entries", checked, len(u.files))
	}
	lines := map[contract.ArchiveKind]int{}
	for _, f := range u.index.Files {
		lines[f.Kind] += f.Lines
	}
	if lines[contract.ArchiveKindSignals] != 2 || lines[contract.ArchiveKindWireCalls] != 8 || u.index.ExportedAt != "2026-10-19T02:00:00.000Z" {
		t.Fatalf("lines %v at %s", lines, u.index.ExportedAt)
	}
}

func TestAnEntryThatIsNotARegularFileRefusesTheArchive(t *testing.T) {
	runDir := bundle(t)
	outside := filepath.Join(t.TempDir(), "run.env")
	if err := os.WriteFile(outside, []byte("TOKEN="+planted+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(runDir, DirReport, "e2e", "result.json")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	_, err := export(t, runDir, masker(t))
	var refusal *Refusal
	if !errors.As(err, &refusal) || !slices.Equal(refusal.Files, []string{"e2e/result.json"}) {
		t.Fatalf("a symbolic link in the bundle: %v", err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	u, err := export(t, runDir, masker(t))
	if err != nil || u.report("summary.json") == "" || u.report("e2e/result.json") != "" {
		t.Fatalf("without the link: %v", err)
	}
}

func TestAFileNoKindNamesAndAPathTheMaskerChangesAreLeftOutAndListedMasked(t *testing.T) {
	runDir := bundle(t)
	put(t, runDir, "notes.txt", "kept by hand: "+planted+"\n")
	put(t, runDir, "source/flows/"+planted+".ts", "export {};\n")
	put(t, runDir, "wire/.partial-suite/calls.jsonl", "")
	put(t, runDir, "wire/.partial-suite/index.json", `{"status":{"take":"suite","state":"taken"},"steps":[],"heads":[]}`)
	put(t, runDir, "wire/lonely/calls.jsonl", "{}\n")
	u, err := export(t, runDir, masker(t, planted))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(u.all, planted) || len(u.files) != 24 || u.report("summary.json") == "" {
		t.Fatalf("%d entries, planted %v", len(u.files), strings.Contains(u.all, planted))
	}
	for name := range u.files {
		if strings.Contains(name, "notes.txt") || strings.Contains(name, "partial") || strings.Contains(name, "lonely") {
			t.Errorf("%s is in the archive", name)
		}
	}
	omitted := strings.Join(u.index.Omitted, "\n")
	for _, want := range []string{"report/notes.txt", "report/source/flows/" + marker, "report/wire/.partial-suite/calls.jsonl", "report/wire/.partial-suite/index.json", "report/wire/lonely/calls.jsonl"} {
		if !strings.Contains(omitted, want) {
			t.Errorf("omitted lacks %s: %q", want, u.index.Omitted)
		}
	}
	if len(u.index.Omitted) != 5 {
		t.Fatalf("omitted %q", u.index.Omitted)
	}
}

func TestTheWireIndexInTheArchivePointsAtTheCallsAsTheyLeave(t *testing.T) {
	runDir := bundle(t)
	stored, err := wire.OpenStore(runDir).Index(wire.SuiteTake)
	if err != nil {
		t.Fatal(err)
	}
	u, err := export(t, runDir, masker(t, planted))
	if err != nil {
		t.Fatal(err)
	}
	var index wire.Index
	if err := json.Unmarshal([]byte(u.report("wire/suite/index.json")), &index); err != nil {
		t.Fatal(err)
	}
	calls := u.report("wire/suite/calls.jsonl")
	moved := 0
	for i, head := range index.Heads {
		var line wire.Line
		if head.Offset+head.Length > int64(len(calls)) || json.Unmarshal([]byte(calls[head.Offset:head.Offset+head.Length]), &line) != nil || line.Call == nil || line.Call.Seq != head.Seq {
			t.Fatalf("head %d does not point at its call", head.Seq)
		}
		if head.Offset != stored.Heads[i].Offset || head.Length != stored.Heads[i].Length {
			moved++
		}
	}
	if len(index.Heads) != 8 || moved == 0 || index.Status.WithheldCalls != 1 {
		t.Fatalf("%d heads, %d moved, status %+v", len(index.Heads), moved, index.Status)
	}
}

func TestTheArchiveHoldsTheSchemaOfEachKindPresentAndOfItsOwnIndex(t *testing.T) {
	runDir := bundle(t)
	for _, rel := range []string{"signals", "e2e/exec"} {
		if err := os.RemoveAll(filepath.Join(runDir, DirReport, rel)); err != nil {
			t.Fatal(err)
		}
	}
	u, err := export(t, runDir, masker(t))
	if err != nil {
		t.Fatal(err)
	}
	var schemas []string
	for name, text := range u.files {
		if rel, ok := strings.CutPrefix(name, runID+"/"+DirSchema+"/"); ok {
			schemas = append(schemas, rel)
			if !strings.Contains(text, `"type"`) {
				t.Errorf("%s is no schema", name)
			}
		}
	}
	slices.Sort(schemas)
	if !slices.Equal(schemas, []string{"archive.json", "context.json", "ledger.json", "manifest.json", "meta.json", "result.json", "summary.json"}) {
		t.Fatalf("schemas %q", schemas)
	}
	described := 0
	for _, f := range u.index.Files {
		if f.Schema != "" {
			described++
			if u.files[runID+"/"+f.Schema] == "" {
				t.Errorf("%s names %s, which the archive does not hold", f.Path, f.Schema)
			}
		}
	}
	if described != 6 {
		t.Fatalf("%d files name a schema", described)
	}
}

func TestARunWithoutASummaryIsNotAnArchive(t *testing.T) {
	empty := t.TempDir()
	if _, err := export(t, empty, masker(t)); !errors.Is(err, ErrNoBundle) {
		t.Fatalf("no report directory: %v", err)
	}
	runDir := bundle(t)
	if err := os.Remove(filepath.Join(runDir, DirReport, "summary.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := export(t, runDir, masker(t)); !errors.Is(err, ErrNoBundle) {
		t.Fatalf("a bundle without its summary: %v", err)
	}
	put(t, runDir, "summary.json", "{}\n")
	if u, err := export(t, runDir, masker(t)); err != nil || u.report("summary.json") != "{}\n" {
		t.Fatalf("with a summary: %v", err)
	}
}

func TestTheSpoolIsUnlinkedAsSoonAsItIsOpen(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	spool, err := OpenSpool()
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	if _, err := spool.Write([]byte("kept only while open")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("the temporary directory holds %d entries: %v", len(entries), err)
	}
	size, err := spool.Rewind()
	back := make([]byte, size)
	if _, readErr := spool.Read(back); err != nil || readErr != nil || string(back) != "kept only while open" {
		t.Fatalf("read back %q: %v %v", back, err, readErr)
	}
	if err := spool.Reset(); err != nil {
		t.Fatal(err)
	}
	if size, err := spool.Rewind(); err != nil || size != 0 {
		t.Fatalf("after reset: %d %v", size, err)
	}
}
