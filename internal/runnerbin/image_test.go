package runnerbin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var imageMutations = []porttest.Mutation{
	{"tag is the binary hash", "Tag returns a fixed string", "red: the tag is not the first 12 chars of the sha256", "TestFirstCallImportsImageTaggedBy12CharSha256WithOnlyRunnerAndTmpInTar"},
	{"non-root user and entrypoint change", "drop both --change flags from the import args", "red: the USER 65532 and ENTRYPOINT assertions fail", "TestFirstCallImportsImageTaggedBy12CharSha256WithOnlyRunnerAndTmpInTar"},
	{"no import when the pin matches", "Ensure always imports", "red: the second call's import count is not 0", "TestMatchingPinSkipsImportAndStaleOrMissingPinImportsAgain"},
	{"re-import when the pin differs", "trust the pin without checking the inspect result", "red: zero imports for a different ID", "TestMatchingPinSkipsImportAndStaleOrMissingPinImportsAgain"},
	{"architecture normalization", "NormalizeArch returns its input unchanged", "red: aarch64 does not become arm64", "TestDaemonArchAliasesNormalizeToArm64OrAmd64AndUnknownArchIsRejected"},
	{"reject a runner that is not embedded", "replace ErrNotEmbedded with an empty binary", "red: an empty image is built without error", "TestPlaceholderOnlyTreeFailsWithRunnerNotEmbeddedAndUnreachableDaemonErrors"},
	{"pin file round trip", "Pin does not write", "red: the reread pin is missing", "TestPinFileIsEmptyWhenMissingAndKeepsEachTagAcrossReads"},
}

type fakeDocker struct {
	arch        string
	failVersion bool
	inspect     map[string]string
	imports     int
	calls       [][]string
	archives    [][]byte
}

func (f *fakeDocker) Run(_ context.Context, stdin io.Reader, args ...string) (string, error) {
	f.calls = append(f.calls, args)
	switch args[0] {
	case "version":
		if f.failVersion {
			return "", errors.New("Cannot connect to the Docker daemon")
		}
		return f.arch, nil
	case "image":
		id, ok := f.inspect[args[len(args)-1]]
		if !ok {
			return "", errors.New("No such image")
		}
		return id, nil
	case "import":
		f.imports++
		data, _ := io.ReadAll(stdin)
		f.archives = append(f.archives, data)
		id := fmt.Sprintf("sha256:%064x", f.imports)
		f.inspect[args[len(args)-1]] = id
		return id, nil
	}
	return "", errors.New("unexpected " + args[0])
}

type memPins struct{ byTag map[string]Image }

func (m *memPins) Pinned(tag string) (string, bool) {
	img, ok := m.byTag[tag]
	return img.ID, ok
}

func (m *memPins) Pin(img Image) error {
	m.byTag[img.Tag] = img
	return nil
}

func fakeRunner(t *testing.T, body string) (func(string) ([]byte, error), []byte) {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(body))
	zw.Close()
	return func(arch string) ([]byte, error) {
		if arch != "arm64" {
			return nil, fmt.Errorf("%w: linux/%s", ErrNotEmbedded, arch)
		}
		return buf.Bytes(), nil
	}, []byte(body)
}

func TestFirstCallImportsImageTaggedBy12CharSha256WithOnlyRunnerAndTmpInTar(t *testing.T) {
	lookup, bin := fakeRunner(t, "ELF-fake-runner")
	d := &fakeDocker{arch: "arm64", inspect: map[string]string{}}
	pins := &memPins{byTag: map[string]Image{}}
	img, err := Ensure(context.Background(), d, lookup, pins)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(bin)
	if img.Tag != "cockpit-runner:"+hex.EncodeToString(sum[:])[:12] || img.ID != fmt.Sprintf("sha256:%064x", 1) || img.Arch != "arm64" || d.imports != 1 {
		t.Fatalf("%+v imports=%d", img, d.imports)
	}
	last := d.calls[len(d.calls)-1]
	joined := strings.Join(last, " ")
	for _, want := range []string{"import", "--platform linux/arm64", `--change ENTRYPOINT ["/cockpit-runner"]`, "--change USER 65532:65532", "- " + img.Tag} {
		if !strings.Contains(joined, want) {
			t.Fatalf("import args lack %q: %q", want, joined)
		}
	}
	tr := tar.NewReader(bytes.NewReader(d.archives[0]))
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
		if h.Name == "cockpit-runner" {
			data, _ := io.ReadAll(tr)
			if h.Mode != 0o755 || !bytes.Equal(data, bin) || h.Typeflag != tar.TypeReg {
				t.Fatalf("runner entry mode=%o type=%v", h.Mode, h.Typeflag)
			}
		}
	}
	if !slices.Equal(names, []string{"tmp/", "cockpit-runner"}) {
		t.Fatalf("tar entries %v", names)
	}
	if pinned, ok := pins.Pinned(img.Tag); !ok || pinned != img.ID {
		t.Fatalf("pin %q %v", pinned, ok)
	}
}

func TestMatchingPinSkipsImportAndStaleOrMissingPinImportsAgain(t *testing.T) {
	lookup, _ := fakeRunner(t, "ELF-fake-runner")
	d := &fakeDocker{arch: "arm64", inspect: map[string]string{}}
	pins := &memPins{byTag: map[string]Image{}}
	first, err := Ensure(context.Background(), d, lookup, pins)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Ensure(context.Background(), d, lookup, pins)
	if err != nil || second != first || d.imports != 1 {
		t.Fatalf("second call: %+v %v imports=%d", second, err, d.imports)
	}
	d.inspect[first.Tag] = "sha256:other"
	third, err := Ensure(context.Background(), d, lookup, pins)
	if err != nil || d.imports != 2 || third.ID == first.ID || third.Tag != first.Tag {
		t.Fatalf("different ID: %+v %v imports=%d", third, err, d.imports)
	}
	if pinned, _ := pins.Pinned(first.Tag); pinned != third.ID {
		t.Fatalf("pin update %q", pinned)
	}
	fresh := &memPins{byTag: map[string]Image{}}
	fourth, err := Ensure(context.Background(), d, lookup, fresh)
	if err != nil || d.imports != 3 || fourth.ID == third.ID {
		t.Fatalf("no pin: %+v %v imports=%d", fourth, err, d.imports)
	}
}

func TestDaemonArchAliasesNormalizeToArm64OrAmd64AndUnknownArchIsRejected(t *testing.T) {
	for raw, want := range map[string]string{"aarch64": "arm64", "arm64\n": "arm64", "x86_64": "amd64", "amd64": "amd64"} {
		if got, err := NormalizeArch(raw); err != nil || got != want {
			t.Fatalf("%q → %q %v", raw, got, err)
		}
	}
	for _, bad := range []string{"", "riscv64", "arm"} {
		if _, err := NormalizeArch(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
	lookup, _ := fakeRunner(t, "x")
	d := &fakeDocker{arch: "aarch64", inspect: map[string]string{}}
	if img, err := Ensure(context.Background(), d, lookup, &memPins{byTag: map[string]Image{}}); err != nil || img.Arch != "arm64" {
		t.Fatalf("%+v %v", img, err)
	}
}

func TestPlaceholderOnlyTreeFailsWithRunnerNotEmbeddedAndUnreachableDaemonErrors(t *testing.T) {
	d := &fakeDocker{arch: "amd64", inspect: map[string]string{}}
	placeholderOnly := func(arch string) ([]byte, error) { return nil, fmt.Errorf("%w: linux/%s", ErrNotEmbedded, arch) }
	_, err := Ensure(context.Background(), d, placeholderOnly, &memPins{byTag: map[string]Image{}})
	if err == nil || !errors.Is(err, ErrNotEmbedded) || !strings.Contains(err.Error(), "not a release build") || d.imports != 0 {
		t.Fatalf("%v imports=%d", err, d.imports)
	}
	lookup, _ := fakeRunner(t, "x")
	down := &fakeDocker{arch: "arm64", failVersion: true, inspect: map[string]string{}}
	if _, err := Ensure(context.Background(), down, lookup, &memPins{byTag: map[string]Image{}}); err == nil || !strings.Contains(err.Error(), "docker") {
		t.Fatalf("%v", err)
	}
}

func TestPinFileIsEmptyWhenMissingAndKeepsEachTagAcrossReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "runner-images.json")
	pins := FilePins{Path: path}
	if id, ok := pins.Pinned("cockpit-runner:abc"); ok || id != "" {
		t.Fatalf("%q %v", id, ok)
	}
	if err := pins.Pin(Image{Tag: "cockpit-runner:abc", ID: "sha256:1", Arch: "arm64"}); err != nil {
		t.Fatal(err)
	}
	if err := pins.Pin(Image{Tag: "cockpit-runner:def", ID: "sha256:2", Arch: "amd64"}); err != nil {
		t.Fatal(err)
	}
	again := FilePins{Path: path}
	if id, ok := again.Pinned("cockpit-runner:abc"); !ok || id != "sha256:1" {
		t.Fatalf("%q %v", id, ok)
	}
	if id, ok := again.Pinned("cockpit-runner:def"); !ok || id != "sha256:2" {
		t.Fatalf("%q %v", id, ok)
	}
}

func TestImageMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "image_test.go", imageMutations)
}
