package headless

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/d0lim/cockpit/internal/archive"
	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/version"
)

func (r *run) targets() []string {
	var out []string
	for _, p := range []string{r.opts.Out, r.opts.Archive} {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (r *run) outputs() (string, error) {
	targets := r.targets()
	if len(targets) == 0 {
		return "no --out or --archive; the bundle goes with the run directory", nil
	}
	for _, p := range targets {
		if _, err := os.Lstat(p); err == nil {
			return "", fmt.Errorf("%s exists and is not replaced", p)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if info, err := os.Stat(filepath.Dir(p)); err != nil || !info.IsDir() {
			return "", fmt.Errorf("the directory of %s does not exist", p)
		}
	}
	return "the bundle goes to " + strings.Join(targets, " and "), nil
}

func (r *run) deliver() {
	if len(r.targets()) == 0 {
		return
	}
	if r.bundle == "" {
		r.st.say("no report bundle: nothing is written to %s", strings.Join(r.targets(), " or "))
		return
	}
	if r.opts.Out != "" {
		r.settle(r.st.run("bundle copy", r.copyBundle))
	}
	if r.opts.Archive != "" {
		r.settle(r.st.run("archive", r.writeArchive))
	}
}

func (r *run) copyBundle() (string, error) {
	if err := os.Mkdir(r.opts.Out, 0o700); err != nil {
		return "", err
	}
	detail, err := r.fillBundleCopy()
	if err != nil {
		os.RemoveAll(r.opts.Out)
	}
	return detail, err
}

func (r *run) fillBundleCopy() (string, error) {
	if err := os.CopyFS(r.opts.Out, os.DirFS(r.bundle)); err != nil {
		return "", err
	}
	files := 0
	err := filepath.WalkDir(r.opts.Out, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.Chmod(p, 0o700)
		}
		files++
		return os.Chmod(p, 0o600)
	})
	return fmt.Sprintf("%s, %d files", r.opts.Out, files), err
}

func (r *run) writeArchive() (string, error) {
	spool, err := archive.OpenSpool()
	if err != nil {
		return "", err
	}
	defer spool.Close()
	index, err := archive.Write(spool, archive.Input{RunDir: r.env.Layout.Root, RunID: r.meta.ID, Mask: r.env.Masker, Now: r.deps.Now(), Version: version.String()})
	if err != nil {
		return "", err
	}
	if _, err := spool.Rewind(); err != nil {
		return "", err
	}
	if _, err := os.Lstat(r.opts.Archive); err == nil {
		return "", fmt.Errorf("%s exists and is not replaced", r.opts.Archive)
	}
	written, err := cli.Save(r.opts.Archive, spool)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s, %d files, %d bytes", r.opts.Archive, len(index.Files), written), nil
}
