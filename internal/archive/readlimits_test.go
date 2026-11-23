package archive

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var importLimitMutations = []porttest.Mutation{
	{"a wire head points at exactly one line of its calls", "drop the head count and head comparison checks in reader.proveTake", "red: the head with a five-gigabyte length unpacks", "TestAWireIndexWhoseHeadsMissTheLinesOfItsCallsIsRefused"},
	{"a JSON file passes the default rules once it is decoded", "return true at the top of reader.decodedClean", "red: the escaped password in meta.json is stored", "TestAnEscapedSecretInAJSONFileIsFoundOnceItIsDecoded"},
	{"an archive holds at most importMaxEntries entries", "drop the entry count check in reader.entry", "red: the many empty files answer a refusal that is no ErrTooLarge", "TestAnArchiveWithMoreEntriesThanTheLimitIsRefused"},
	{"the gzip stream is read to its end and checked", "skip the trailing copy in Unpack", "red: the stream without its footer unpacks", "TestAGzipStreamCutShortCorruptOrWithDataAfterTheTarIsRefused"},
}

func TestImportLimitMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "readlimits_test.go", importLimitMutations)
}

func TestAWireIndexWhoseHeadsMissTheLinesOfItsCallsIsRefused(t *testing.T) {
	base := exported(t)
	calls := entryBody(base, "wire/suite/calls.jsonl")
	if _, _, err := importInto(t, tarOf(t, withTake(t, base, calls, nil))); err != nil {
		t.Fatalf("the rebuilt take is refused: %v", err)
	}
	cases := map[string]func([]map[string]any){
		"huge length":  func(h []map[string]any) { h[0]["length"] = 5 << 30 },
		"negative":     func(h []map[string]any) { h[0]["length"] = -1 },
		"shifted":      func(h []map[string]any) { h[len(h)-1]["offset"] = h[len(h)-1]["offset"].(int) + 1 },
		"other seq":    func(h []map[string]any) { h[0]["seq"] = 999 },
		"missing head": func(h []map[string]any) { h[len(h)-1]["drop"] = true },
	}
	refused := 0
	for name, edit := range cases {
		list := withTake(t, base, calls, edit)
		_, _, err := importInto(t, tarOf(t, list))
		var invalid *Invalid
		if !errors.As(err, &invalid) || !strings.Contains(invalid.Reason, "wire index") {
			t.Errorf("%s: want the index refused, got %v", name, err)
			continue
		}
		refused++
	}
	if refused != 5 {
		t.Fatalf("refused %d of %d", refused, len(cases))
	}
}

func resize(path, body string) func(*contract.Archive) {
	return func(a *contract.Archive) {
		for i := range a.Files {
			if a.Files[i].Path == path {
				a.Files[i].Bytes = int64(len(body))
			}
		}
	}
}

func TestAnEscapedSecretInAJSONFileIsFoundOnceItIsDecoded(t *testing.T) {
	base := exported(t)
	meta := entryBody(base, "meta.json")
	escaped := strings.Replace(meta, `"suite":"toy"`, `"suite":"toy","notice":"\u0070assword: `+importPlanted+`"`, 1)
	if !strings.Contains(escaped, `\u0070assword`) {
		t.Fatalf("the escape was not planted: %s", escaped)
	}
	clean := strings.Replace(meta, `"suite":"toy"`, `"suite":"toy","notice":"path: flows"`, 1)
	if _, _, err := importInto(t, tarOf(t, indexed(t, replaced(base, "meta.json", clean), resize("report/meta.json", clean)))); err != nil {
		t.Fatalf("an escape of harmless text is refused: %v", err)
	}
	_, _, err := importInto(t, tarOf(t, indexed(t, replaced(base, "meta.json", escaped), resize("report/meta.json", escaped))))
	var unmasked *Unmasked
	if !errors.As(err, &unmasked) || !slices.Equal(unmasked.Files, []string{"report/meta.json"}) || strings.Contains(err.Error(), importPlanted) {
		t.Fatalf("want meta.json refused, got %v", err)
	}
}

func TestAnArchiveWithMoreEntriesThanTheLimitIsRefused(t *testing.T) {
	list := exported(t)
	for i := range importMaxEntries {
		list = append(list, entry{name: fmt.Sprintf("%s/report/source/empty/%d.ts", runID, i)})
	}
	_, _, err := importInto(t, tarOf(t, list))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
}

func TestAGzipStreamCutShortCorruptOrWithDataAfterTheTarIsRefused(t *testing.T) {
	raw := tarOf(t, exported(t))
	if _, _, err := importInto(t, raw); err != nil {
		t.Fatalf("the whole stream is refused: %v", err)
	}
	corrupt := slices.Clone(raw)
	corrupt[len(corrupt)-8] ^= 0xff
	var trailing bytes.Buffer
	z := gzip.NewWriter(&trailing)
	z.Write(make([]byte, importTrailBytes+1))
	z.Close()
	for name, body := range map[string][]byte{"cut short": raw[:len(raw)-8], "bad checksum": corrupt} {
		_, _, err := importInto(t, body)
		var invalid *Invalid
		if !errors.As(err, &invalid) {
			t.Errorf("%s: want a refusal, got %v", name, err)
		}
	}
	if _, _, err := importInto(t, append(slices.Clone(raw), trailing.Bytes()...)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("data after the tar: want ErrTooLarge, got %v", err)
	}
}
