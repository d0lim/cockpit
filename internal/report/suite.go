package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	BundleSource    = "source"
	SourceIndexFile = "source/source-index.json"
)

type SourceIndex struct {
	Files   map[string]SourceEntry `json:"files"`
	Omitted map[string]string      `json:"omitted"`
}

type SourceEntry struct {
	Lines int `json:"lines"`
}

func sourceFiles(m *contract.Manifest) []string {
	var out []string
	for _, f := range m.Flows {
		if f.Source != nil {
			out = append(out, f.Source.File)
		}
		for _, s := range f.Steps {
			if s.Source != nil {
				out = append(out, s.Source.File)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func SuiteSources(suiteDir string, m *contract.Manifest) (map[string][]byte, contract.CollectStage) {
	files := map[string][]byte{}
	if m == nil {
		return files, contract.CollectStage{Status: contract.CollectStageSkipped, Detail: "no manifest"}
	}
	wanted := sourceFiles(m)
	if len(wanted) == 0 {
		return files, contract.CollectStage{Status: contract.CollectStageAbsent, Detail: "the manifest has no source locations"}
	}
	root, err := os.OpenRoot(suiteDir)
	if err != nil {
		return files, contract.CollectStage{Status: contract.CollectStageError, Detail: "the suite directory cannot be opened"}
	}
	defer root.Close()
	index := SourceIndex{Files: map[string]SourceEntry{}, Omitted: map[string]string{}}
	for _, name := range wanted {
		if !filepath.IsLocal(name) || BundleSource+"/"+filepath.ToSlash(name) == SourceIndexFile {
			index.Omitted[name] = "not a path inside the suite"
			continue
		}
		data, failure := readCapped(root, name)
		if failure != ReadOK {
			index.Omitted[name] = string(failure)
			continue
		}
		files[BundleSource+"/"+filepath.ToSlash(name)] = data
		index.Files[name] = SourceEntry{Lines: bytes.Count(data, []byte("\n")) + 1}
	}
	encoded, _ := json.MarshalIndent(index, "", "  ")
	files[SourceIndexFile] = append(encoded, '\n')
	if len(index.Omitted) > 0 {
		return files, contract.CollectStage{Status: contract.CollectStageError, Detail: "some source files were omitted"}
	}
	return files, contract.CollectStage{Status: contract.CollectStageOK}
}
