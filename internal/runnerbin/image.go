package runnerbin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/d0lim/cockpit/internal/version"
)

const (
	Repository = "cockpit-runner"
	runnerUser = "65532:65532"
	binaryName = "cockpit-runner"
)

type Docker interface {
	Run(ctx context.Context, stdin io.Reader, args ...string) (string, error)
}

type CLI struct{}

func (CLI) Run(ctx context.Context, stdin io.Reader, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

type Image struct {
	Tag  string `json:"tag"`
	ID   string `json:"id"`
	Arch string `json:"arch"`
}

type Pins interface {
	Pinned(tag string) (string, bool)
	Pin(img Image) error
}

func NormalizeArch(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "arm64", "aarch64":
		return "arm64", nil
	case "amd64", "x86_64":
		return "amd64", nil
	}
	return "", fmt.Errorf("unsupported daemon architecture %q", strings.TrimSpace(raw))
}

var (
	hubRevision    = version.Revision
	runnerRevision = readRevision
)

func readRevision(bin []byte) (string, error) {
	info, err := buildinfo.Read(bytes.NewReader(bin))
	if err != nil {
		return "", err
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value, nil
		}
	}
	return "", nil
}

func checkRevision(bin []byte) error {
	hub := hubRevision()
	if hub == "" {
		return nil
	}
	runner, err := runnerRevision(bin)
	if err != nil {
		return fmt.Errorf("cannot read the embedded runner's build info: %w", err)
	}
	if runner == "" || runner == hub {
		return nil
	}
	return fmt.Errorf("the embedded runner was built from %s but this cockpit was built from %s; run scripts/build-runners.sh, then rebuild cockpit (make build does both)", short(runner), short(hub))
}

func short(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

func Tag(bin []byte) string {
	sum := sha256.Sum256(bin)
	return Repository + ":" + hex.EncodeToString(sum[:])[:12]
}

func Ensure(ctx context.Context, d Docker, lookup func(goarch string) ([]byte, error), pins Pins) (Image, error) {
	raw, err := d.Run(ctx, nil, "version", "--format", "{{.Server.Arch}}")
	if err != nil {
		return Image{}, fmt.Errorf("cannot reach the docker daemon: %w", err)
	}
	arch, err := NormalizeArch(raw)
	if err != nil {
		return Image{}, err
	}
	gz, err := lookup(arch)
	if errors.Is(err, ErrNotEmbedded) {
		return Image{}, fmt.Errorf("this cockpit is not a release build (scripts/build-runners.sh): %w", err)
	}
	if err != nil {
		return Image{}, err
	}
	bin, err := gunzip(gz)
	if err != nil {
		return Image{}, fmt.Errorf("cannot decompress the embedded runner: %w", err)
	}
	if err := checkRevision(bin); err != nil {
		return Image{}, err
	}
	tag := Tag(bin)
	if pinned, ok := pins.Pinned(tag); ok {
		if current, err := d.Run(ctx, nil, "image", "inspect", "--format", "{{.Id}}", tag); err == nil && current == pinned {
			return Image{Tag: tag, ID: pinned, Arch: arch}, nil
		}
	}
	id, err := importImage(ctx, d, bin, tag, arch)
	if err != nil {
		return Image{}, err
	}
	img := Image{Tag: tag, ID: id, Arch: arch}
	if err := pins.Pin(img); err != nil {
		return Image{}, fmt.Errorf("cannot write the runner image pin: %w", err)
	}
	return img, nil
}

func importImage(ctx context.Context, d Docker, bin []byte, tag, arch string) (string, error) {
	archive, err := tarball(bin)
	if err != nil {
		return "", err
	}
	out, err := d.Run(ctx, bytes.NewReader(archive), "import", "--platform", "linux/"+arch,
		"--change", `ENTRYPOINT ["/cockpit-runner"]`, "--change", "USER "+runnerUser, "-", tag)
	if err != nil {
		return "", fmt.Errorf("cannot create the runner image: %w", err)
	}
	id := strings.TrimSpace(out)
	if !strings.HasPrefix(id, "sha256:") {
		return "", fmt.Errorf("docker import did not return an image ID: %q", id)
	}
	return id, nil
}

func tarball(bin []byte) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	entries := []*tar.Header{
		{Typeflag: tar.TypeDir, Name: "tmp/", Mode: 0o1777},
		{Typeflag: tar.TypeReg, Name: binaryName, Mode: 0o755, Size: int64(len(bin))},
	}
	for _, h := range entries {
		if err := tw.WriteHeader(h); err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write(bin); err != nil {
				return nil, err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzip(gz []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
