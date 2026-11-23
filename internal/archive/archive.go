package archive

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/d0lim/cockpit"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	maxTextBytes  = 64 << 20
	maxIndexBytes = 512 << 20
	fileMode      = 0o600
)

var ErrNoBundle = errors.New("the run has no report bundle")

type Refusal struct{ Files []string }

func (r *Refusal) Error() string {
	return "the report bundle holds an entry that is not a regular file"
}

type Input struct {
	RunDir  string
	RunID   string
	Mask    *mask.Masker
	Now     time.Time
	Version string
}

type writer struct {
	in      Input
	root    *os.Root
	tar     *tar.Writer
	spool   *Spool
	index   contract.Archive
	schemas map[string]bool
}

func Write(out io.Writer, in Input) (contract.Archive, error) {
	if in.Mask == nil {
		return contract.Archive{}, errors.New("archive: no masker")
	}
	root, err := os.OpenRoot(filepath.Join(in.RunDir, DirReport))
	if errors.Is(err, fs.ErrNotExist) {
		return contract.Archive{}, ErrNoBundle
	}
	if err != nil {
		return contract.Archive{}, err
	}
	defer root.Close()
	files, odd, err := scan(root)
	if err != nil {
		return contract.Archive{}, err
	}
	if len(odd) > 0 {
		for i, name := range odd {
			odd[i] = in.Mask.Mask(name)
		}
		return contract.Archive{}, &Refusal{Files: odd}
	}
	if !slices.Contains(files, report.FileSummary) {
		return contract.Archive{}, ErrNoBundle
	}
	spool, err := OpenSpool()
	if err != nil {
		return contract.Archive{}, err
	}
	defer spool.Close()
	zipped := gzip.NewWriter(out)
	w := &writer{in: in, root: root, tar: tar.NewWriter(zipped), spool: spool, schemas: map[string]bool{SchemaIndex: true}}
	w.index = contract.Archive{
		Format: contract.ArchiveFormat1, RunID: in.RunID, ExportedAt: store.Timestamp(in.Now), CockpitVersion: in.Version,
		Files: []contract.ArchiveFile{}, Omitted: []string{},
	}
	if err := w.bundle(files); err != nil {
		return contract.Archive{}, err
	}
	if err := w.finish(); err != nil {
		return contract.Archive{}, err
	}
	if err := w.tar.Close(); err != nil {
		return contract.Archive{}, err
	}
	return w.index, zipped.Close()
}

func (w *writer) bundle(files []string) error {
	takes := map[string]int{}
	for _, rel := range files {
		kind, known := KindOf(rel)
		take, inTake := takeOf(rel)
		wired := known && (kind.Name == contract.ArchiveKindWireIndex || kind.Name == contract.ArchiveKindWireCalls)
		var err error
		switch {
		case !known, w.in.Mask.Mask(rel) != rel, wired && !inTake:
			w.omit(rel)
		case wired:
			takes[take]++
		case kind.Name == contract.ArchiveKindSignals:
			err = w.signals(rel, kind)
		default:
			err = w.text(rel, kind)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", w.in.Mask.Mask(rel), err)
		}
	}
	names := make([]string, 0, len(takes))
	for name := range takes {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		dir := wire.StagingDir + "/" + name + "/"
		if takes[name] != 2 {
			for _, rel := range []string{dir + wire.CallsFile, dir + wire.IndexFile} {
				if slices.Contains(files, rel) {
					w.omit(rel)
				}
			}
			continue
		}
		if err := w.take(dir); err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
	}
	return nil
}

func (w *writer) omit(rel string) {
	w.index.Omitted = append(w.index.Omitted, w.in.Mask.Mask(DirReport+"/"+rel))
}

func (w *writer) text(rel string, kind Kind) error {
	data, err := readRegular(w.root, rel, maxTextBytes)
	if err != nil {
		return err
	}
	return w.entry(DirReport+"/"+rel, kind, []byte(w.in.Mask.Mask(string(data))))
}

func (w *writer) signals(rel string, kind Kind) error {
	f, err := openRegular(w.root, rel)
	if err != nil {
		return err
	}
	defer f.Close()
	buffered := bufio.NewWriter(w.spool)
	lines, err := signal.Remask(w.in.Mask, f, buffered)
	if err == nil {
		err = buffered.Flush()
	}
	if err != nil {
		return err
	}
	return w.spooled(DirReport+"/"+rel, kind, lines)
}

func (w *writer) take(dir string) error {
	raw, err := readRegular(w.root, dir+wire.IndexFile, maxIndexBytes)
	if err != nil {
		return err
	}
	var index wire.Index
	if err := json.Unmarshal(raw, &index); err != nil {
		return errors.New("the index of the take cannot be read")
	}
	calls, err := openRegular(w.root, dir+wire.CallsFile)
	if err != nil {
		return err
	}
	defer calls.Close()
	buffered := bufio.NewWriter(w.spool)
	leaving, err := wire.Export(w.in.Mask, index, calls, buffered)
	if err == nil {
		err = buffered.Flush()
	}
	if err != nil {
		return err
	}
	callsKind, _ := KindOf(dir + wire.CallsFile)
	if err := w.spooled(DirReport+"/"+dir+wire.CallsFile, callsKind, len(leaving.Heads)); err != nil {
		return err
	}
	data, err := json.Marshal(leaving)
	if err != nil {
		return err
	}
	indexKind, _ := KindOf(dir + wire.IndexFile)
	return w.entry(DirReport+"/"+dir+wire.IndexFile, indexKind, append(data, '\n'))
}

func (w *writer) finish() error {
	names := make([]string, 0, len(w.schemas))
	for name := range w.schemas {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		at := DirSchema + "/" + name + schemaSuffix
		data, err := cockpit.Schemas.ReadFile(at)
		if err != nil {
			return err
		}
		if err := w.entry(at, Kind{Name: contract.ArchiveKindSchema}, data); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(w.index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := w.header(FileIndex, int64(len(data))); err != nil {
		return err
	}
	_, err = w.tar.Write(data)
	return err
}
