package compose

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	PrivateEnvName = "e2e.private.env.json"
	emptyPrivEnv   = "{\n  \"local\": {}\n}\n"
	publicDirMode  = 0o755
	publicFileMode = 0o644
	execFileMode   = 0o755
)

func Extract(ctx context.Context, git Git, tree Tree, root *os.Root) error {
	for _, dir := range []string{srcDir, buildDir} {
		if err := root.Mkdir(dir, publicDirMode); err != nil {
			return fmt.Errorf("cannot create %s: %w", dir, err)
		}
		pr, pw := io.Pipe()
		done := make(chan error, 1)
		go func() {
			err := git.Archive(ctx, tree.SHA, pw)
			pw.CloseWithError(err)
			done <- err
		}()
		n, err := untar(root, dir, pr)
		if err == nil {
			_, err = io.Copy(io.Discard, pr)
		}
		pr.CloseWithError(errors.New("extraction stopped"))
		if archiveErr := <-done; err == nil && archiveErr != nil {
			err = archiveErr
		}
		if err != nil {
			return fmt.Errorf("cannot extract commit %s into %s: %w", tree.SHA[:12], dir, err)
		}
		if want := tree.Files(); n != want {
			return fmt.Errorf("extracted %d files into %s, but commit %s has %d", n, dir, tree.SHA[:12], want)
		}
	}
	return nil
}

func untar(root *os.Root, dir string, r io.Reader) (int, error) {
	tr := tar.NewReader(r)
	files := 0
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return files, err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := path.Clean(h.Name)
		if !filepath.IsLocal(name) {
			return files, fmt.Errorf("archive entry %q is outside the tree", h.Name)
		}
		target := path.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			err = root.MkdirAll(target, publicDirMode)
		case tar.TypeReg:
			err = writeEntry(root, target, h.FileInfo().Mode(), tr)
			files++
		case tar.TypeSymlink:
			if err = root.MkdirAll(path.Dir(target), publicDirMode); err == nil {
				err = root.Symlink(h.Linkname, target)
			}
			files++
		default:
			err = fmt.Errorf("archive entry %q has unsupported type %q", h.Name, h.Typeflag)
		}
		if err != nil {
			return files, err
		}
	}
}

func writeEntry(root *os.Root, target string, mode fs.FileMode, r io.Reader) error {
	if err := root.MkdirAll(path.Dir(target), publicDirMode); err != nil {
		return err
	}
	perm := fs.FileMode(publicFileMode)
	if mode&0o111 != 0 {
		perm = execFileMode
	}
	f, err := root.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func MakeWorkTree(root *os.Root, decl contract.Declaration) error {
	if err := privateDir(root, workDir); err != nil {
		return err
	}
	if err := copyTree(root, srcPath(decl.Scripts.Dir), path.Join(workDir, "scripts")); err != nil {
		return err
	}
	if err := copyTree(root, srcPath(DeclarationPath), path.Join(workDir, DeclarationPath)); err != nil {
		return err
	}
	for _, s := range decl.Inputs.RunnerSources {
		if err := copyTree(root, srcPath(s.Path), path.Join(workDir, "source", string(s.Name))); err != nil {
			return err
		}
	}
	priv := []byte(emptyPrivEnv)
	for _, p := range decl.Env.Outputs {
		if path.Base(string(p)) != PrivateEnvName {
			continue
		}
		data, err := readRegular(root, srcPath(p))
		if err != nil {
			return err
		}
		priv = data
	}
	return writePrivate(root, path.Join(workDir, "e2e", PrivateEnvName), priv)
}

func readRegular(root *os.Root, name string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	return root.ReadFile(name)
}

func copyTree(root *os.Root, from, to string) error {
	return fs.WalkDir(root.FS(), from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		target := path.Join(to, filepath.ToSlash(rel))
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("refusing to copy the symbolic link %s into the runner work tree", p)
		case d.IsDir():
			return root.MkdirAll(target, publicDirMode)
		case !d.Type().IsRegular():
			return fmt.Errorf("refusing to copy %s: it is not a regular file", p)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		f, err := root.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		return writeEntry(root, target, info.Mode(), f)
	})
}
