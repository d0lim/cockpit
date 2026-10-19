package runfiles

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"syscall"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	E2EDir       = "work/e2e"
	ExecDir      = "exec"
	MaxBytes     = 4 << 20
	manifest     = "manifest.json"
	result       = "result.json"
	ledger       = "ledger.json"
	ContainerE2E = "/work/e2e"
)

var execName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.json$`)

type Kind string

const (
	Missing  Kind = "missing"
	Unsafe   Kind = "unsafe"
	TooLarge Kind = "too-large"
	Parse    Kind = "parse"
)

type Error struct {
	Kind   Kind
	Path   string
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s: %s", e.Path, e.Kind)
	}
	return fmt.Sprintf("%s: %s (%s)", e.Path, e.Kind, e.Detail)
}

func IsMissing(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Kind == Missing
}

func Read(root *os.Root, rel string, max int64) ([]byte, error) {
	if !filepath.IsLocal(rel) {
		return nil, &Error{Kind: Unsafe, Path: rel, Detail: "not a path inside the run directory"}
	}
	info, err := root.Lstat(rel)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, &Error{Kind: Missing, Path: rel}
	case err != nil:
		return nil, &Error{Kind: Unsafe, Path: rel, Detail: "cannot be inspected inside the run directory"}
	case !info.Mode().IsRegular():
		return nil, &Error{Kind: Unsafe, Path: rel, Detail: "not a regular file"}
	case info.Size() > max:
		return nil, &Error{Kind: TooLarge, Path: rel, Detail: fmt.Sprintf("%d bytes over %d", info.Size(), max)}
	}
	f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, &Error{Kind: Unsafe, Path: rel, Detail: "cannot be opened without following links"}
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return nil, &Error{Kind: Unsafe, Path: rel, Detail: "not a regular file"}
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, &Error{Kind: Unsafe, Path: rel, Detail: "cannot be read"}
	}
	if int64(len(data)) > max {
		return nil, &Error{Kind: TooLarge, Path: rel, Detail: fmt.Sprintf("over %d bytes", max)}
	}
	return data, nil
}

func readWith[T any](root *os.Root, rel string, decode func(io.Reader) (*T, error)) (*T, error) {
	data, err := Read(root, rel, MaxBytes)
	if err != nil {
		return nil, err
	}
	v, err := decode(bytes.NewReader(data))
	if err != nil {
		return nil, &Error{Kind: Parse, Path: rel, Detail: err.Error()}
	}
	return v, nil
}

func Manifest(root *os.Root) (*contract.Manifest, error) {
	return readWith(root, path.Join(E2EDir, manifest), contract.ReadRunnerFile[contract.Manifest])
}

func Result(root *os.Root) (*contract.Result, error) {
	return readWith(root, path.Join(E2EDir, result), contract.ReadRunnerFile[contract.Result])
}

func Ledger(root *os.Root) (*contract.Ledger, error) {
	l, err := readWith(root, path.Join(E2EDir, ledger), contract.ReadRunnerFile[contract.Ledger])
	if IsMissing(err) {
		return &contract.Ledger{SchemaVersion: contract.RunnerSchemaVersion, Flows: map[string]contract.LedgerEntry{}}, nil
	}
	return l, err
}

func ValidExecName(name string) bool { return execName.MatchString(name) }

func Exec(root *os.Root, name string) (*contract.ExecResult, error) {
	if !ValidExecName(name) {
		return nil, &Error{Kind: Unsafe, Path: name, Detail: "not an exec result name"}
	}
	return readWith(root, path.Join(E2EDir, ExecDir, name), contract.ReadRunnerFile[contract.ExecResult])
}
