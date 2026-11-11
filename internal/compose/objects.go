package compose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
)

var objectsOnly = []string{"--git-dir=.", "--work-tree=."}

type objects struct {
	dir string
}

func (o objects) stream(ctx context.Context, w io.Writer, args ...string) error {
	return Git{Dir: o.dir}.stream(ctx, w, objectsOnly, args)
}

func (o objects) run(ctx context.Context, args ...string) ([]byte, error) {
	var out bytes.Buffer
	if err := o.stream(ctx, &out, args...); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (o objects) list(ctx context.Context, commit string) (map[string]Entry, error) {
	out, err := o.run(ctx, "ls-tree", "-r", "-t", "-z", "--full-tree", commit)
	if err != nil {
		return nil, err
	}
	entries := map[string]Entry{}
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		meta, path, ok := strings.Cut(rec, "\t")
		parts := strings.Fields(meta)
		if !ok || len(parts) != 3 {
			return nil, fmt.Errorf("unexpected ls-tree record %q", rec)
		}
		entries[path] = Entry{Mode: parts[0], Type: parts[1], ID: parts[2]}
	}
	return entries, nil
}

func (o objects) archive(ctx context.Context, commit, prefix string, w io.Writer) error {
	if !shaPattern.MatchString(commit) {
		return fmt.Errorf("%w: archive needs a resolved sha, got %q", ErrCommitInput, commit)
	}
	args := []string{"archive", "--format=tar"}
	if prefix != "" {
		args = append(args, "--prefix="+prefix+"/")
	}
	return o.stream(ctx, w, append(args, commit)...)
}

func (o objects) blob(ctx context.Context, id string) ([]byte, error) {
	if !shaPattern.MatchString(id) {
		return nil, fmt.Errorf("not an object id: %q", id)
	}
	var buf bytes.Buffer
	w := &capWriter{buf: &buf, max: maxBlobBytes}
	if err := o.stream(ctx, w, "cat-file", "blob", id); err != nil {
		return nil, err
	}
	if w.over {
		return nil, fmt.Errorf("object %s exceeds %d bytes", id[:12], maxBlobBytes)
	}
	return buf.Bytes(), nil
}

type capWriter struct {
	buf  *bytes.Buffer
	max  int
	over bool
}

func (w *capWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		w.over = true
		return len(p), nil
	}
	return w.buf.Write(p)
}
