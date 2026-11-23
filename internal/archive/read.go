package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
)

const (
	ImportMaxBytes    = 128 << 20
	importFileBytes   = 64 << 20
	importTotalBytes  = 512 << 20
	importIndexBytes  = 16 << 20
	importSchemaBytes = 1 << 20
	importMaxEntries  = 10000
	importTrailBytes  = 1 << 20
)

var ErrTooLarge = errors.New("the archive is over the import limits")

type Invalid struct {
	Reason string
	Files  []string
	cause  error
}

func (e *Invalid) Error() string { return "not a bundle archive the hub writes: " + e.Reason }

func (e *Invalid) Unwrap() error { return e.cause }

type Unmasked struct{ Files []string }

func (e *Unmasked) Error() string {
	return "the default masking rules still change files of the archive"
}

type Imported struct {
	RunID string
	Index contract.Archive
	Meta  contract.BundleMeta
	Files int
}

type reader struct {
	m      *mask.Masker
	root   *os.Root
	runID  string
	names  map[string]bool
	report map[string]int64
	schema map[string]int64
	index  []byte
	total  int64
	left   []string
}

func Unpack(body io.Reader, stage string, m *mask.Masker) (Imported, error) {
	if m == nil {
		return Imported{}, errors.New("archive: no masker")
	}
	root, err := os.OpenRoot(stage)
	if err != nil {
		return Imported{}, err
	}
	defer root.Close()
	zipped, err := gzip.NewReader(body)
	if err != nil {
		return Imported{}, &Invalid{Reason: "the body is not gzip", cause: err}
	}
	r := &reader{m: m, root: root, names: map[string]bool{}, report: map[string]int64{}, schema: map[string]int64{}}
	t := tar.NewReader(zipped)
	for {
		h, err := t.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Imported{}, &Invalid{Reason: "the body is not a whole tar.gz", cause: err}
		}
		if err := r.entry(h, t); err != nil {
			return Imported{}, err
		}
	}
	trail, err := io.Copy(io.Discard, io.LimitReader(zipped, importTrailBytes+1))
	if err != nil {
		return Imported{}, &Invalid{Reason: "the body is not a whole tar.gz", cause: err}
	}
	if trail > importTrailBytes {
		return Imported{}, fmt.Errorf("data after the tar end: %w", ErrTooLarge)
	}
	return r.finish()
}

func (r *reader) invalid(reason, name string) error {
	return &Invalid{Reason: reason, Files: []string{r.m.Mask(name)}}
}

func (r *reader) entry(h *tar.Header, body io.Reader) error {
	id, rest, ok := strings.Cut(h.Name, "/")
	if !ok || !store.IsRunID(id) || (r.runID != "" && id != r.runID) {
		return r.invalid("every entry sits under one directory named after a run", h.Name)
	}
	r.runID = id
	if len(r.names) >= importMaxEntries {
		return fmt.Errorf("more than %d entries: %w", importMaxEntries, ErrTooLarge)
	}
	if h.Typeflag != tar.TypeReg {
		return r.invalid("an entry is not a regular file", h.Name)
	}
	if rest == "" || path.Clean(rest) != rest || !filepath.IsLocal(rest) || strings.ContainsAny(rest, "\\\x00") {
		return r.invalid("an entry path does not stay inside its run directory", h.Name)
	}
	if r.names[rest] {
		return r.invalid("an entry appears twice", h.Name)
	}
	r.names[rest] = true
	switch {
	case rest == FileIndex:
		data, err := r.read(h, body, importIndexBytes)
		r.index = data
		return err
	case strings.HasPrefix(rest, DirSchema+"/"):
		if ok, _ := path.Match(DirSchema+"/*"+schemaSuffix, rest); !ok {
			return r.invalid("a schema entry is not schema/<name>.json", h.Name)
		}
		_, err := r.read(h, body, importSchemaBytes)
		r.schema[rest] = h.Size
		return err
	case strings.HasPrefix(rest, DirReport+"/"):
		return r.reportFile(h, body, strings.TrimPrefix(rest, DirReport+"/"))
	}
	return r.invalid("an entry is neither index.json, a schema nor a report file", h.Name)
}

func (r *reader) reportFile(h *tar.Header, body io.Reader, rel string) error {
	kind, known := KindOf(rel)
	if !known {
		return r.invalid("a report file names no kind of the bundle", h.Name)
	}
	if wired(kind) {
		if _, ok := takeOf(rel); !ok {
			return r.invalid("a wire file is not inside a take", h.Name)
		}
	}
	data, err := r.read(h, body, importFileBytes)
	if err != nil {
		return err
	}
	r.report[rel] = h.Size
	text := string(data)
	clean, err := r.decodedClean(rel, kind, data)
	if err != nil {
		return err
	}
	if r.m.Mask(rel) != rel || !clean || kind.Name != contract.ArchiveKindWireCalls && r.m.Mask(text) != text {
		r.left = append(r.left, r.m.Mask(DirReport+"/"+rel))
		return nil
	}
	return r.store(rel, data)
}

func wired(k Kind) bool {
	return k.Name == contract.ArchiveKindWireIndex || k.Name == contract.ArchiveKindWireCalls
}

func (r *reader) read(h *tar.Header, body io.Reader, limit int64) ([]byte, error) {
	r.total += h.Size
	if h.Size < 0 || h.Size > limit || r.total > importTotalBytes {
		return nil, fmt.Errorf("%s: %w", r.m.Mask(h.Name), ErrTooLarge)
	}
	data := make([]byte, h.Size)
	if _, err := io.ReadFull(body, data); err != nil {
		return nil, &Invalid{Reason: "the body is not a whole tar.gz", cause: err}
	}
	return data, nil
}

func (r *reader) store(rel string, data []byte) error {
	target := DirReport + "/" + rel
	if err := r.root.MkdirAll(path.Dir(target), 0o700); err != nil {
		return err
	}
	f, err := r.root.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

func (r *reader) finish() (Imported, error) {
	if r.runID == "" {
		return Imported{}, &Invalid{Reason: "the archive is empty"}
	}
	if r.index == nil {
		return Imported{}, &Invalid{Reason: "the archive has no index.json"}
	}
	if text := string(r.index); r.m.Mask(text) != text {
		r.left = append(r.left, FileIndex)
	}
	if err := r.proveTakes(); err != nil {
		return Imported{}, err
	}
	if len(r.left) > 0 {
		slices.Sort(r.left)
		return Imported{}, &Unmasked{Files: r.left}
	}
	index, err := r.checkIndex()
	if err != nil {
		return Imported{}, err
	}
	meta, err := r.checkBundle()
	if err != nil {
		return Imported{}, err
	}
	return Imported{RunID: r.runID, Index: index, Meta: meta, Files: len(r.report)}, nil
}
