package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"syscall"

	"github.com/d0lim/cockpit/internal/contract"
)

const MaxStateBytes = 4 << 20

const (
	RunnerManifest = "manifest.json"
	RunnerResult   = "result.json"
	RunnerLedger   = "ledger.json"
	RunnerContext  = "context.json"
	RunnerExecDir  = "exec"
	BundleRunner   = "e2e"
)

var execName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.json$`)

var (
	errUnsafe   = errors.New("not a regular file inside the directory, or over the size limit")
	stepStatus  = []contract.StepStatus{contract.StepStatusNotRun, contract.StepStatusRunning, contract.StepStatusPassed, contract.StepStatusFailed, contract.StepStatusSkipped}
	stepKinds   = []contract.StepKind{contract.StepKindRequest, contract.StepKindWait, contract.StepKindCapture}
	stageRunner = "runner."
)

type Collected struct {
	Manifest ManifestRead
	Result   ResultRead
	Files    map[string][]byte
	Stages   map[string]contract.CollectStage
}

func ReadRunnerFiles(dir string) Collected {
	c := Collected{Files: map[string][]byte{}, Stages: map[string]contract.CollectStage{}}
	root, err := os.OpenRoot(dir)
	if err != nil {
		c.Manifest.Failure, c.Result.Failure = ReadMissing, ReadMissing
		for _, name := range []string{RunnerManifest, RunnerResult, RunnerLedger, RunnerContext, RunnerExecDir} {
			c.Stages[stageRunner+name] = contract.CollectStage{Status: contract.CollectStageAbsent}
		}
		return c
	}
	defer root.Close()
	raw := map[string][]byte{}
	fails := map[string]ReadFailure{}
	for _, name := range []string{RunnerManifest, RunnerResult, RunnerLedger, RunnerContext} {
		raw[name], fails[name] = readCapped(root, name)
		c.Stages[stageRunner+name] = stageOf(fails[name])
		if fails[name] == ReadOK {
			c.Files[BundleRunner+"/"+name] = raw[name]
		}
	}
	c.Stages[stageRunner+RunnerExecDir] = readExec(root, c.Files)
	c.Manifest = decodeManifest(raw[RunnerManifest], fails[RunnerManifest])
	c.Result = decodeResult(raw[RunnerResult], fails[RunnerResult])
	return c
}

func stageOf(f ReadFailure) contract.CollectStage {
	switch f {
	case ReadOK:
		return contract.CollectStage{Status: contract.CollectStageOK}
	case ReadMissing:
		return contract.CollectStage{Status: contract.CollectStageAbsent}
	}
	return contract.CollectStage{Status: contract.CollectStageError, Detail: string(f)}
}

func readCapped(root *os.Root, name string) ([]byte, ReadFailure) {
	link, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ReadMissing
	}
	if err != nil || !link.Mode().IsRegular() {
		return nil, ReadUnsafe
	}
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, ReadUnsafe
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !os.SameFile(link, info) || info.Size() > MaxStateBytes {
		return nil, ReadUnsafe
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxStateBytes+1))
	if err != nil || len(data) > MaxStateBytes {
		return nil, ReadUnsafe
	}
	return data, ReadOK
}

func readExec(root *os.Root, files map[string][]byte) contract.CollectStage {
	entries, err := fs.ReadDir(root.FS(), RunnerExecDir)
	if errors.Is(err, fs.ErrNotExist) {
		return contract.CollectStage{Status: contract.CollectStageAbsent}
	}
	if err != nil {
		return contract.CollectStage{Status: contract.CollectStageError, Detail: errUnsafe.Error()}
	}
	odd := 0
	for _, e := range entries {
		if !execName.MatchString(e.Name()) {
			odd++
			continue
		}
		data, failure := readCapped(root, RunnerExecDir+"/"+e.Name())
		if failure != ReadOK {
			odd++
			continue
		}
		files[BundleRunner+"/"+RunnerExecDir+"/"+e.Name()] = data
	}
	if odd > 0 {
		return contract.CollectStage{Status: contract.CollectStageError, Detail: "some exec files were not copied"}
	}
	return contract.CollectStage{Status: contract.CollectStageOK}
}

func decodeFailure(err error) ReadFailure {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) || errors.Is(err, contract.ErrTrailingData) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return ReadParse
	}
	return ReadSchema
}

func decodeManifest(data []byte, failure ReadFailure) ManifestRead {
	if failure != ReadOK {
		return ManifestRead{Failure: failure}
	}
	m, err := contract.ReadRunnerFile[contract.Manifest](bytes.NewReader(data))
	if err != nil {
		return ManifestRead{Failure: decodeFailure(err)}
	}
	for _, f := range m.Flows {
		for _, s := range f.Steps {
			if !slices.Contains(stepKinds, s.Kind) {
				return ManifestRead{Failure: ReadSchema}
			}
		}
	}
	return ManifestRead{Value: m}
}

func decodeResult(data []byte, failure ReadFailure) ResultRead {
	if failure != ReadOK {
		return ResultRead{Failure: failure}
	}
	r, err := contract.ReadRunnerFile[contract.Result](bytes.NewReader(data))
	if err != nil {
		return ResultRead{Failure: decodeFailure(err)}
	}
	for _, s := range r.Steps {
		if !slices.Contains(stepStatus, s.Status) {
			return ResultRead{Failure: ReadSchema}
		}
	}
	return ResultRead{Value: r}
}
