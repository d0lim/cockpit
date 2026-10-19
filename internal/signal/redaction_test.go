package signal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var redactionMutations = []porttest.Mutation{
	{"append masks string leaves before writing", "write json.Marshal(sig) instead of maskSignal", "red: the planted value is in stream.jsonl", "TestPlantedValueInAnEventBodyIsMaskedInTheStoreTheReadAndTheFreezeCopy"},
	{"read masks again with the current value list", "return the stored line without remask", "red: a value added to the list after append shows in the read", "TestReadMasksAgainWithTheCurrentValueListAddedAfterAppend"},
	{"the freeze copy masks again", "copy raw lines in Freeze", "red: a value added before freeze shows in the bundle copy", "TestReadMasksAgainWithTheCurrentValueListAddedAfterAppend"},
	{"append without a masker is refused", "skip the masker check in Append", "red: a signal is written with no masker", "TestAppendAndOpenWithoutAMaskerAreRefusedAndWriteNothing"},
	{"credential-named attributes are hidden whole", "Leaves ignores key names", "red: the token attribute keeps its value", "TestCredentialNamedAttributesAreHiddenWholeAndOtherAttributesStay"},
}

func TestPlantedValueInAnEventBodyIsMaskedInTheStoreTheReadAndTheFreezeCopy(t *testing.T) {
	dir := runDir(t)
	s := openAt(t, dir, DefaultCaps(), masker(t, planted))
	mustAppend(t, s, event("toy-db", "row id=7 note="+planted, contract.Attrs{"table": "orders", "cell": planted}))
	bundle := filepath.Join(t.TempDir(), "report.partial")
	if _, err := s.Freeze(bundle); err != nil {
		t.Fatal(err)
	}
	resp := readAll(t, s, 0)
	read := resp.Signals[0].Event.Body + " " + resp.Signals[0].Attrs["cell"].(string)
	places := map[string]string{
		"store":  fileText(t, filepath.Join(dir, Dir, streamFile)),
		"read":   read,
		"freeze": fileText(t, filepath.Join(bundle, Dir, streamFile)),
	}
	for where, text := range places {
		if strings.Contains(text, planted) || !strings.Contains(text, "****") {
			t.Errorf("%s: planted value present or marker absent: %q", where, text)
		}
	}
	if len(places) != 3 {
		t.Fatal("place count")
	}
}

func TestReadMasksAgainWithTheCurrentValueListAddedAfterAppend(t *testing.T) {
	late := "LATE-issued-credential-55aa"
	current := masker(t)
	dir := runDir(t)
	s, err := Open(dir, Options{Masker: func() *mask.Masker { return current }, Now: clock()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	mustAppend(t, s, event("toy-db", "issued "+late, nil))
	if !strings.Contains(fileText(t, filepath.Join(dir, Dir, streamFile)), late) {
		t.Fatal("canary: the value was masked before it was known")
	}
	current = masker(t, late)
	body := readAll(t, s, 0).Signals[0].Event.Body
	if strings.Contains(body, late) || !strings.Contains(body, "****") {
		t.Fatalf("read %q", body)
	}
	bundle := t.TempDir()
	if _, err := s.Freeze(bundle); err != nil {
		t.Fatal(err)
	}
	if copied := fileText(t, filepath.Join(bundle, Dir, streamFile)); strings.Contains(copied, late) || !strings.Contains(copied, "****") {
		t.Fatalf("freeze copy %q", copied)
	}
}

func TestAppendAndOpenWithoutAMaskerAreRefusedAndWriteNothing(t *testing.T) {
	if _, err := Open(runDir(t), Options{}); !errors.Is(err, ErrNoMasker) {
		t.Fatalf("open: %v", err)
	}
	dir := runDir(t)
	var current *mask.Masker
	s, err := Open(dir, Options{Masker: func() *mask.Masker { return current }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Append(event("toy-db", planted, nil)); !errors.Is(err, ErrNoMasker) {
		t.Fatalf("append: %v", err)
	}
	if info, _ := os.Stat(filepath.Join(dir, Dir, streamFile)); info.Size() != 0 {
		t.Fatalf("stream has %d bytes", info.Size())
	}
	current = masker(t, planted)
	if seq, err := s.Append(event("toy-db", "ok", nil)); err != nil || seq != 1 {
		t.Fatalf("canary: %d %v", seq, err)
	}
}

func TestCredentialNamedAttributesAreHiddenWholeAndOtherAttributesStay(t *testing.T) {
	s := openAt(t, runDir(t), DefaultCaps(), masker(t))
	mustAppend(t, s, event("toy-db", "", contract.Attrs{"access_token": "short", "rows": 3.0, "label": "visible"}))
	got := readAll(t, s, 0).Signals[0].Attrs
	if got["access_token"] != "****" || got["rows"] != 3.0 || got["label"] != "visible" {
		t.Fatalf("%v", got)
	}
}

func TestRedactionMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "redaction_test.go", redactionMutations)
}
