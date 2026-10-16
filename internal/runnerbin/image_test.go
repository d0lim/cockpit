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
	{"태그는 바이너리 해시", "Tag 가 고정 문자열을 돌려준다", "빨강: 태그가 sha256 앞 12자가 아니다", "Test첫_호출은_import_하고_태그는_바이너리_sha256_앞_12자이며_tar_에는_0755_러너_하나와_tmp_뿐이다"},
	{"비 root·entrypoint 변경", "import 인자에서 --change 둘을 지운다", "빨강: USER 65532 와 ENTRYPOINT 단정이 깨진다", "Test첫_호출은_import_하고_태그는_바이너리_sha256_앞_12자이며_tar_에는_0755_러너_하나와_tmp_뿐이다"},
	{"핀과 같으면 import 안 함", "Ensure 가 항상 import 한다", "빨강: 둘째 호출의 import 횟수가 0 이 아니다", "Test핀이_태그를_가리키면_import_하지_않고_다른_ID_를_가리키거나_핀이_없으면_다시_import_한다"},
	{"핀과 다르면 다시 import", "inspect 결과를 보지 않고 핀만 믿는다", "빨강: 다른 ID 에서 import 0회", "Test핀이_태그를_가리키면_import_하지_않고_다른_ID_를_가리키거나_핀이_없으면_다시_import_한다"},
	{"아키텍처 정규화", "NormalizeArch 가 입력을 그대로 돌려준다", "빨강: aarch64 가 arm64 가 아니다", "Test데몬_아키텍처_aarch64_와_x86_64_는_arm64_amd64_로_정규화되고_모르는_값은_거부된다"},
	{"러너 미내장 거부", "ErrNotEmbedded 를 빈 바이너리로 대신한다", "빨강: 오류 없이 빈 이미지를 만든다", "Test러너_자리표시만_있는_트리에서는_러너_미내장으로_끝나고_데몬에_닿지_못해도_오류다"},
	{"핀 파일 왕복", "Pin 이 쓰지 않는다", "빨강: 다시 읽은 핀이 없다", "Test핀_파일은_없으면_비어_있고_태그별로_남아_다시_읽힌다"},
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

func Test첫_호출은_import_하고_태그는_바이너리_sha256_앞_12자이며_tar_에는_0755_러너_하나와_tmp_뿐이다(t *testing.T) {
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
			t.Fatalf("import 인자에 %q 가 없다: %q", want, joined)
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
				t.Fatalf("러너 항목 mode=%o type=%v", h.Mode, h.Typeflag)
			}
		}
	}
	if !slices.Equal(names, []string{"tmp/", "cockpit-runner"}) {
		t.Fatalf("tar 항목 %v", names)
	}
	if pinned, ok := pins.Pinned(img.Tag); !ok || pinned != img.ID {
		t.Fatalf("핀 %q %v", pinned, ok)
	}
}

func Test핀이_태그를_가리키면_import_하지_않고_다른_ID_를_가리키거나_핀이_없으면_다시_import_한다(t *testing.T) {
	lookup, _ := fakeRunner(t, "ELF-fake-runner")
	d := &fakeDocker{arch: "arm64", inspect: map[string]string{}}
	pins := &memPins{byTag: map[string]Image{}}
	first, err := Ensure(context.Background(), d, lookup, pins)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Ensure(context.Background(), d, lookup, pins)
	if err != nil || second != first || d.imports != 1 {
		t.Fatalf("둘째 호출: %+v %v imports=%d", second, err, d.imports)
	}
	d.inspect[first.Tag] = "sha256:other"
	third, err := Ensure(context.Background(), d, lookup, pins)
	if err != nil || d.imports != 2 || third.ID == first.ID || third.Tag != first.Tag {
		t.Fatalf("다른 ID: %+v %v imports=%d", third, err, d.imports)
	}
	if pinned, _ := pins.Pinned(first.Tag); pinned != third.ID {
		t.Fatalf("핀 갱신 %q", pinned)
	}
	fresh := &memPins{byTag: map[string]Image{}}
	fourth, err := Ensure(context.Background(), d, lookup, fresh)
	if err != nil || d.imports != 3 || fourth.ID == third.ID {
		t.Fatalf("핀 없음: %+v %v imports=%d", fourth, err, d.imports)
	}
}

func Test데몬_아키텍처_aarch64_와_x86_64_는_arm64_amd64_로_정규화되고_모르는_값은_거부된다(t *testing.T) {
	for raw, want := range map[string]string{"aarch64": "arm64", "arm64\n": "arm64", "x86_64": "amd64", "amd64": "amd64"} {
		if got, err := NormalizeArch(raw); err != nil || got != want {
			t.Fatalf("%q → %q %v", raw, got, err)
		}
	}
	for _, bad := range []string{"", "riscv64", "arm"} {
		if _, err := NormalizeArch(bad); err == nil {
			t.Fatalf("%q 가 통과했다", bad)
		}
	}
	lookup, _ := fakeRunner(t, "x")
	d := &fakeDocker{arch: "aarch64", inspect: map[string]string{}}
	if img, err := Ensure(context.Background(), d, lookup, &memPins{byTag: map[string]Image{}}); err != nil || img.Arch != "arm64" {
		t.Fatalf("%+v %v", img, err)
	}
}

func Test러너_자리표시만_있는_트리에서는_러너_미내장으로_끝나고_데몬에_닿지_못해도_오류다(t *testing.T) {
	d := &fakeDocker{arch: "amd64", inspect: map[string]string{}}
	placeholderOnly := func(arch string) ([]byte, error) { return nil, fmt.Errorf("%w: linux/%s", ErrNotEmbedded, arch) }
	_, err := Ensure(context.Background(), d, placeholderOnly, &memPins{byTag: map[string]Image{}})
	if err == nil || !errors.Is(err, ErrNotEmbedded) || !strings.Contains(err.Error(), "러너 미내장") || d.imports != 0 {
		t.Fatalf("%v imports=%d", err, d.imports)
	}
	lookup, _ := fakeRunner(t, "x")
	down := &fakeDocker{arch: "arm64", failVersion: true, inspect: map[string]string{}}
	if _, err := Ensure(context.Background(), down, lookup, &memPins{byTag: map[string]Image{}}); err == nil || !strings.Contains(err.Error(), "docker") {
		t.Fatalf("%v", err)
	}
}

func Test핀_파일은_없으면_비어_있고_태그별로_남아_다시_읽힌다(t *testing.T) {
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

func Test이미지_변이_기록표는_이_파일의_시험과_양방향으로_같다(t *testing.T) {
	porttest.AssertMutationRecord(t, "image_test.go", imageMutations)
}
