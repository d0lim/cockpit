package archive

import (
	"archive/zip"
	"bytes"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var unwrapMutations = []porttest.Mutation{
	{"a zip is opened and its one archive unpacked", "send every body to Unpack in UnpackFile", "red: the artifact zip is refused as not gzip", "TestTheZipOfAnArtifactUnpacksLikeTheArchiveItHolds"},
	{"a zip holds exactly one file", "take the first file whatever the count in theArchive", "red: the zip with a second file unpacks", "TestAZipThatIsNotOneStoredOrDeflatedArchiveIsRefused"},
	{"the file in the zip is a regular *.tar.gz", "drop the mode and suffix check in theArchive", "red: the zip holding notes.txt answers a refusal of the tar instead", "TestAZipThatIsNotOneStoredOrDeflatedArchiveIsRefused"},
	{"the file in the zip is stored or deflated", "drop the method check in theArchive", "red: the entry with method 99 answers an open error instead", "TestAZipThatIsNotOneStoredOrDeflatedArchiveIsRefused"},
	{"the file in the zip is within the body limit unpacked", "drop the size check in theArchive", "red: the entry that declares 129 MiB answers a refusal that is no ErrTooLarge", "TestAZipThatIsNotOneStoredOrDeflatedArchiveIsRefused"},
	{"a name in a refused zip is masked", "list the zip names as they are in theArchive", "red: files carries the planted token", "TestAZipThatIsNotOneStoredOrDeflatedArchiveIsRefused"},
}

func TestUnwrapMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "unwrap_test.go", unwrapMutations)
}

type zipped struct {
	name   string
	body   []byte
	method uint16
	size   uint64
}

func zipOf(t *testing.T, files ...zipped) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range files {
		if f.size != 0 || f.method > zip.Deflate {
			h := &zip.FileHeader{Name: f.name, Method: f.method, CRC32: crc32.ChecksumIEEE(f.body), CompressedSize64: uint64(len(f.body)), UncompressedSize64: f.size}
			raw, err := w.CreateRaw(h)
			if err != nil {
				t.Fatal(err)
			}
			raw.Write(f.body)
			continue
		}
		h := &zip.FileHeader{Name: f.name, Method: f.method}
		if strings.HasSuffix(f.name, "/") {
			h.SetMode(os.ModeDir | 0o700)
		}
		entry, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write(f.body)
	}
	w.Close()
	return out.Bytes()
}

func unpackFile(t *testing.T, raw []byte) (Imported, string, error) {
	t.Helper()
	stage := filepath.Join(t.TempDir(), ".import-test")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := UnpackFile(bytes.NewReader(raw), stage, masker(t))
	return got, stage, err
}

func TestTheZipOfAnArtifactUnpacksLikeTheArchiveItHolds(t *testing.T) {
	archive := tarOf(t, exported(t))
	direct, _, err := unpackFile(t, archive)
	if err != nil {
		t.Fatalf("the archive itself is refused: %v", err)
	}
	for _, method := range []uint16{zip.Deflate, zip.Store} {
		got, stage, err := unpackFile(t, zipOf(t, zipped{name: "cockpit-run.tar.gz", body: archive, method: method}))
		if err != nil {
			t.Fatalf("method %d: %v", method, err)
		}
		raw, readErr := os.ReadFile(filepath.Join(stage, DirReport, "summary.json"))
		if got.RunID != runID || got.Files != direct.Files || readErr != nil || !strings.Contains(string(raw), runID) {
			t.Fatalf("method %d: imported %+v, summary %v", method, got, readErr)
		}
	}
}

func TestAZipThatIsNotOneStoredOrDeflatedArchiveIsRefused(t *testing.T) {
	archive := tarOf(t, exported(t))
	one := zipped{name: "cockpit-run.tar.gz", body: archive, method: zip.Deflate}
	cases := map[string]struct {
		raw     []byte
		tooBig  bool
		planted bool
	}{
		"two files":      {raw: zipOf(t, one, zipped{name: "token=" + importPlanted + ".tar.gz", body: archive, method: zip.Deflate}), planted: true},
		"empty":          {raw: zipOf(t)},
		"a directory":    {raw: zipOf(t, zipped{name: "cockpit-run.tar.gz/", method: zip.Store})},
		"another name":   {raw: zipOf(t, zipped{name: "notes.txt", body: archive, method: zip.Deflate})},
		"another method": {raw: zipOf(t, zipped{name: "cockpit-run.tar.gz", body: archive, method: 99})},
		"declared huge":  {raw: zipOf(t, zipped{name: "cockpit-run.tar.gz", body: archive, method: zip.Store, size: ImportMaxBytes + 1}), tooBig: true},
		"neither":        {raw: []byte("plain text, no magic")},
	}
	refused := 0
	for name, c := range cases {
		_, stage, err := unpackFile(t, c.raw)
		var invalid *Invalid
		switch {
		case c.tooBig && !errors.Is(err, ErrTooLarge):
			t.Errorf("%s: want ErrTooLarge, got %v", name, err)
			continue
		case !c.tooBig && (!errors.As(err, &invalid) || !strings.HasPrefix(invalid.Reason, "the zip") && !strings.HasPrefix(invalid.Reason, "the file")):
			t.Errorf("%s: want the zip refused, got %v", name, err)
			continue
		}
		if c.planted && (strings.Contains(strings.Join(invalid.Files, " "), importPlanted) || !strings.Contains(strings.Join(invalid.Files, " "), "****")) {
			t.Errorf("%s: the names are not masked: %q", name, invalid.Files)
		}
		if names, _ := os.ReadDir(stage); len(names) != 0 {
			t.Errorf("%s: the stage holds %v", name, names)
		}
		refused++
	}
	if refused != 7 {
		t.Fatalf("refused %d of %d", refused, len(cases))
	}
}
