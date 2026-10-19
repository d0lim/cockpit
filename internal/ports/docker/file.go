package dockerport

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/ports"
)

type FileRequest struct {
	Service  string `json:"service"`
	Path     string `json:"path"`
	MaxBytes int    `json:"maxBytes,omitempty"`
}

type tarRead struct {
	content []byte
	over    bool
	files   int
	links   int
	err     error
}

type cpDone struct {
	res compose.Result
	err error
}

func (p *Port) File(ctx context.Context, req FileRequest) (TextOutput, error) {
	if !strings.HasPrefix(req.Path, "/") || strings.ContainsRune(req.Path, 0) || len(req.Path) > 4096 {
		return TextOutput{}, ports.BadArgs(p.mask, "path must be an absolute path inside the container")
	}
	id, err := p.resolve(ctx, req.Service)
	if err != nil {
		return TextOutput{}, err
	}
	max := p.bytesFor(req.MaxBytes)
	cctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	stop, halt := context.WithCancel(cctx)
	defer halt()
	pr, pw := io.Pipe()
	done := make(chan cpDone, 1)
	go func() {
		res, err := p.docker.Run(stop, compose.Command{Args: []string{"cp", id + ":" + req.Path, "-"}, Stdout: pw})
		_ = pw.CloseWithError(io.EOF)
		done <- cpDone{res, err}
	}()
	got := readTar(pr, rawLimit(max), p.maxEntries)
	halt()
	_ = pr.CloseWithError(io.ErrClosedPipe)
	cp := <-done
	at := "docker cp " + req.Service + ":" + req.Path
	switch {
	case errors.Is(cctx.Err(), context.DeadlineExceeded):
		return TextOutput{}, p.failure(cctx, cctx.Err(), cp.res, p.timeout, at)
	case got.over:
		return clip(got.content, true, max, p.mask), nil
	case got.files == 0 && got.links > 0:
		return TextOutput{}, ports.Denied(p.mask, at+" is a symbolic link and links are not followed")
	case cp.err != nil || cp.res.Code != 0:
		return TextOutput{}, p.failure(cctx, cp.err, cp.res, p.timeout, at)
	case got.err != nil:
		return TextOutput{}, ports.Unavailable(p.mask, at+" did not return a readable tar stream")
	case got.files == 0:
		return TextOutput{}, ports.BadArgs(p.mask, at+" is not a regular file")
	}
	return clip(got.content, false, max, p.mask), nil
}

func readTar(r io.Reader, limit, maxEntries int) tarRead {
	var out tarRead
	tr := tar.NewReader(r)
	for entries := 0; ; entries++ {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			out.err = err
			return out
		}
		if entries == maxEntries {
			out.over = true
			return out
		}
		switch hdr.Typeflag {
		case tar.TypeSymlink, tar.TypeLink:
			out.links++
			continue
		case tar.TypeReg:
		default:
			continue
		}
		out.files++
		room := limit - len(out.content)
		chunk, err := io.ReadAll(io.LimitReader(tr, int64(room)+1))
		if err != nil {
			out.err = err
			return out
		}
		if len(chunk) > room {
			out.content = append(out.content, chunk[:room]...)
			out.over = true
			return out
		}
		out.content = append(out.content, chunk...)
	}
}
