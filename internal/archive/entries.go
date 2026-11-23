package archive

import (
	"archive/tar"
	"io"
	"io/fs"
	"os"
	"slices"

	"github.com/d0lim/cockpit/internal/contract"
)

func scan(root *os.Root) (files, odd []string, err error) {
	err = fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		switch {
		case err != nil && rel == ".":
			return err
		case err != nil:
			odd = append(odd, rel)
		case d.IsDir():
		case d.Type().IsRegular():
			files = append(files, rel)
		default:
			odd = append(odd, rel)
		}
		return nil
	})
	slices.Sort(files)
	slices.Sort(odd)
	return files, odd, err
}

func (w *writer) header(at string, size int64) error {
	return w.tar.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: w.in.RunID + "/" + at, Mode: fileMode, Size: size, ModTime: w.in.Now})
}

func (w *writer) list(at string, kind Kind, size int64, lines int) {
	if kind.Schema != "" {
		w.schemas[kind.Schema] = true
	}
	w.index.Files = append(w.index.Files, contract.ArchiveFile{Path: at, Kind: kind.Name, Bytes: size, Lines: lines, Schema: kind.schemaPath()})
}

func (w *writer) entry(at string, kind Kind, data []byte) error {
	if err := w.header(at, int64(len(data))); err != nil {
		return err
	}
	if _, err := w.tar.Write(data); err != nil {
		return err
	}
	w.list(at, kind, int64(len(data)), 0)
	return nil
}

func (w *writer) spooled(at string, kind Kind, lines int) error {
	size, err := w.spool.Rewind()
	if err != nil {
		return err
	}
	if err := w.header(at, size); err != nil {
		return err
	}
	if _, err := io.CopyN(w.tar, w.spool, size); err != nil {
		return err
	}
	w.list(at, kind, size, lines)
	return w.spool.Reset()
}
