package store

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func ApplyHubUmask() int {
	return syscall.Umask(0o077)
}

func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

type writeAll func(f *os.File, data []byte) error

func writeFully(f *os.File, data []byte) error {
	_, err := f.Write(data)
	return err
}

func WriteAtomic(path string, data []byte) error {
	return writeAtomic(path, data, writeFully)
}

func writeAtomic(path string, data []byte, write writeAll) error {
	tmp := path + "." + strconv.Itoa(os.Getpid()) + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	err = f.Chmod(0o600)
	if err == nil {
		err = write(f, data)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Clean(path))
}
