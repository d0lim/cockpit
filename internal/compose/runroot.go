package compose

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
)

func OpenRun(runDir string) (*os.Root, error) {
	if err := os.MkdirAll(runDir, privateDirMod); err != nil {
		return nil, err
	}
	if err := os.Chmod(runDir, privateDirMod); err != nil {
		return nil, err
	}
	return os.OpenRoot(runDir)
}

func privateDir(root *os.Root, name string) error {
	if err := root.MkdirAll(name, privateDirMod); err != nil {
		return err
	}
	return root.Chmod(name, privateDirMod)
}

func writePrivate(root *os.Root, name string, data []byte) error {
	if dir := path.Dir(name); dir != "." {
		if err := privateDir(root, dir); err != nil {
			return err
		}
	}
	info, err := root.Lstat(name)
	switch {
	case err == nil && !info.Mode().IsRegular():
		return fmt.Errorf("refusing to write %s: it is not a regular file", name)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, privateFile)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return root.Chmod(name, privateFile)
}
