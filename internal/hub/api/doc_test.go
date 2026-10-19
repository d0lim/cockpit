package api

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

var docRow = regexp.MustCompile("^\\| (GET|POST) \\| `([^`]+)` \\|")

func TestAPIDocumentRouteTableMatchesTheRegisteredRoutesBothWays(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "api.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	section := text[strings.Index(text, "## Routes"):strings.Index(text, "### Response shapes")]
	var documented []string
	for _, line := range strings.Split(section, "\n") {
		if m := docRow.FindStringSubmatch(line); m != nil {
			documented = append(documented, m[1]+" "+m[2])
		}
	}
	registered := Routes()
	slices.Sort(documented)
	slices.Sort(registered)
	if !slices.Equal(documented, registered) || len(registered) != 12 {
		t.Fatalf("documented %q\nregistered %q", documented, registered)
	}
}
