package archive

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/d0lim/cockpit/internal/mask"
)

var (
	gzipMagic = []byte{0x1f, 0x8b}
	zipMagic  = []byte("PK\x03\x04")
)

const archiveSuffix = ".tar.gz"

func UnpackFile(body io.Reader, stage string, m *mask.Masker) (Imported, error) {
	if m == nil {
		return Imported{}, errors.New("archive: no masker")
	}
	peek := bufio.NewReader(body)
	head, _ := peek.Peek(len(zipMagic))
	switch {
	case bytes.HasPrefix(head, gzipMagic):
		return Unpack(peek, stage, m)
	case bytes.HasPrefix(head, zipMagic):
		inner, done, err := unzipOne(peek, m)
		if err != nil {
			return Imported{}, err
		}
		defer done()
		return Unpack(inner, stage, m)
	}
	return Imported{}, &Invalid{Reason: "the file is neither a tar.gz nor a zip that holds one"}
}

func unzipOne(body io.Reader, m *mask.Masker) (io.Reader, func(), error) {
	spool, err := OpenSpool()
	if err != nil {
		return nil, nil, err
	}
	size, err := io.Copy(spool, io.LimitReader(body, ImportMaxBytes+1))
	if err != nil {
		spool.Close()
		return nil, nil, &Invalid{Reason: "the zip did not arrive whole", cause: err}
	}
	if size > ImportMaxBytes {
		spool.Close()
		return nil, nil, fmt.Errorf("the zip: %w", ErrTooLarge)
	}
	z, err := zip.NewReader(spool, size)
	if err != nil {
		spool.Close()
		return nil, nil, &Invalid{Reason: "the zip does not read as one", cause: err}
	}
	f, err := theArchive(z, m)
	if err != nil {
		spool.Close()
		return nil, nil, err
	}
	inner, err := f.Open()
	if err != nil {
		spool.Close()
		return nil, nil, &Invalid{Reason: "the archive in the zip does not open", Files: []string{m.Mask(f.Name)}, cause: err}
	}
	done := func() {
		inner.Close()
		spool.Close()
	}
	return io.LimitReader(inner, ImportMaxBytes), done, nil
}

func theArchive(z *zip.Reader, m *mask.Masker) (*zip.File, error) {
	if len(z.File) != 1 {
		names := make([]string, 0, min(len(z.File), 10))
		for _, f := range z.File[:min(len(z.File), 10)] {
			names = append(names, m.Mask(f.Name))
		}
		return nil, &Invalid{Reason: "the zip must hold exactly one file, the bundle archive", Files: names}
	}
	f := z.File[0]
	switch {
	case !f.Mode().IsRegular() || !strings.HasSuffix(f.Name, archiveSuffix):
		return nil, &Invalid{Reason: "the file in the zip is not a regular file named *.tar.gz", Files: []string{m.Mask(f.Name)}}
	case f.Method != zip.Store && f.Method != zip.Deflate:
		return nil, &Invalid{Reason: "the file in the zip is neither stored nor deflated", Files: []string{m.Mask(f.Name)}}
	case f.UncompressedSize64 > ImportMaxBytes:
		return nil, fmt.Errorf("%s: %w", m.Mask(f.Name), ErrTooLarge)
	}
	return f, nil
}
