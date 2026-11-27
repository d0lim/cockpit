package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/store"
)

func TestTheBundleMetadataNamesADeclarationOtherThanTheDefaultAndOmitsTheDefault(t *testing.T) {
	encode := func(m store.Meta) string {
		raw, err := json.Marshal(BundleMetaOf(m, SummaryInput{}, contract.VerdictPassed))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	if got := encode(store.Meta{ID: "run-a", Declaration: ".cockpit/alt.yaml"}); !strings.Contains(got, `"declaration":".cockpit/alt.yaml"`) {
		t.Fatalf("the metadata of an alt run: %s", got)
	}
	if got := encode(store.Meta{ID: "run-a"}); strings.Contains(got, "declaration") || !strings.Contains(got, `"id":"run-a"`) {
		t.Fatalf("the metadata of a default run: %s", got)
	}
}
