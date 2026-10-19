package signal

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var keyMutations = []porttest.Mutation{
	{"attribute keys are masked in the store, the read and the freeze copy", "keep the original key in mask.maskKeys", "red: the planted attribute key is in stream.jsonl, the read and the bundle copy", "TestAPlantedAttributeKeyIsMaskedInTheStoreTheReadAndTheFreezeCopy"},
}

func TestSignalKeyMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "keys_test.go", keyMutations)
}

func TestAPlantedAttributeKeyIsMaskedInTheStoreTheReadAndTheFreezeCopy(t *testing.T) {
	dir := runDir(t)
	s := openAt(t, dir, DefaultCaps(), masker(t, planted))
	mustAppend(t, s, event("toy-db", "row", contract.Attrs{planted: "seen", "table": "orders"}))
	bundle := filepath.Join(t.TempDir(), "report.partial")
	if _, err := s.Freeze(bundle); err != nil {
		t.Fatal(err)
	}
	resp := readAll(t, s, 0)
	read, err := json.Marshal(resp.Signals[0].Attrs)
	if err != nil {
		t.Fatal(err)
	}
	places := map[string]string{
		"store":  fileText(t, filepath.Join(dir, Dir, streamFile)),
		"read":   string(read),
		"freeze": fileText(t, filepath.Join(bundle, Dir, streamFile)),
	}
	for where, text := range places {
		if strings.Contains(text, planted) || !strings.Contains(text, "****") || !strings.Contains(text, `"table":"orders"`) {
			t.Errorf("%s: %s", where, text)
		}
	}
	if len(places) != 3 {
		t.Fatal(len(places))
	}
}
