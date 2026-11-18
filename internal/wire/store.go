package wire

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	SuiteTake     = "suite"
	StagingDir    = "wire"
	BundleDir     = "report"
	CallsFile     = "calls.jsonl"
	IndexFile     = "index.json"
	partialPrefix = ".partial-"
	dirMode       = 0o700
	fileMode      = 0o600
)

var takeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func ValidTake(name string) bool { return takeName.MatchString(name) }

type Store struct {
	runDir string
}

func OpenStore(runDir string) Store { return Store{runDir: runDir} }

func (s Store) staging() string { return filepath.Join(s.runDir, StagingDir) }

func (s Store) bundle() string { return filepath.Join(s.runDir, BundleDir) }

func (s Store) attached() string { return filepath.Join(s.bundle(), StagingDir) }

func (s Store) Root() string {
	if isDir(s.staging()) || !isDir(s.bundle()) {
		return s.staging()
	}
	return s.attached()
}

func (s Store) Detach() error {
	if err := os.MkdirAll(s.staging(), dirMode); err != nil {
		return err
	}
	return moveTakes(s.attached(), s.staging())
}

func (s Store) Attach() error {
	if !isDir(s.bundle()) {
		return nil
	}
	return moveTakes(s.staging(), s.attached())
}

func (s Store) Takes() ([]string, error) {
	entries, err := os.ReadDir(s.Root())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && ValidTake(e.Name()) && isFile(filepath.Join(s.Root(), e.Name(), IndexFile)) {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

func (s Store) Index(take string) (Index, error) {
	if !ValidTake(take) {
		return Index{}, fmt.Errorf("not a take name: %w", fs.ErrNotExist)
	}
	data, err := os.ReadFile(filepath.Join(s.Root(), take, IndexFile))
	if err != nil {
		return Index{}, err
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return Index{}, fmt.Errorf("the index of take %s cannot be read: %w", take, err)
	}
	return index, nil
}

func (s Store) Call(take string, seq int64) (Call, Head, error) {
	index, err := s.Index(take)
	if err != nil {
		return Call{}, Head{}, err
	}
	at := slices.IndexFunc(index.Heads, func(h Head) bool { return h.Seq == seq })
	if at < 0 {
		return Call{}, Head{}, fmt.Errorf("take %s has no call %d: %w", take, seq, fs.ErrNotExist)
	}
	call, err := s.CallAt(take, index.Heads[at])
	return call, index.Heads[at], err
}

func (s Store) CallAt(take string, head Head) (Call, error) {
	if !ValidTake(take) {
		return Call{}, fmt.Errorf("not a take name: %w", fs.ErrNotExist)
	}
	f, err := os.Open(filepath.Join(s.Root(), take, CallsFile))
	if err != nil {
		return Call{}, err
	}
	defer f.Close()
	text := make([]byte, head.Length)
	if _, err := f.ReadAt(text, head.Offset); err != nil {
		return Call{}, fmt.Errorf("call %d of take %s cannot be read: %w", head.Seq, take, err)
	}
	var line Line
	if err := json.Unmarshal(text, &line); err != nil || line.Call == nil || line.Call.Seq != head.Seq {
		return Call{}, fmt.Errorf("call %d of take %s does not match its index", head.Seq, take)
	}
	return *line.Call, nil
}

type Version struct {
	Modified int64
	Size     int64
}

func (s Store) IndexVersion(take string) (Version, error) {
	if !ValidTake(take) {
		return Version{}, fmt.Errorf("not a take name: %w", fs.ErrNotExist)
	}
	info, err := os.Lstat(filepath.Join(s.Root(), take, IndexFile))
	if err != nil {
		return Version{}, err
	}
	return Version{Modified: info.ModTime().UnixNano(), Size: info.Size()}, nil
}

func (s Store) stored(take string) (Status, bool) {
	index, err := s.Index(take)
	if err != nil || index.Status.State != StateTaken {
		return Status{}, false
	}
	return index.Status, true
}

func moveTakes(from, to string) error {
	entries, err := os.ReadDir(from)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		source, target := filepath.Join(from, e.Name()), filepath.Join(to, e.Name())
		if strings.HasPrefix(e.Name(), partialPrefix) || exists(target) {
			if err := os.RemoveAll(source); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(to, dirMode); err != nil {
			return err
		}
		if err := os.Rename(source, target); err != nil {
			return err
		}
	}
	return os.Remove(from)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}
