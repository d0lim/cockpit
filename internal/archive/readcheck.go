package archive

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/report"
)

var finalVerdicts = []contract.FinalVerdict{contract.VerdictPassed, contract.VerdictFailed, contract.VerdictError, contract.VerdictAborted}

func strict(data []byte, into any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(into); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return &Invalid{Reason: "a JSON file holds more than one value"}
	}
	return nil
}

func (r *reader) checkIndex() (contract.Archive, error) {
	var index contract.Archive
	if err := strict(r.index, &index); err != nil {
		return contract.Archive{}, &Invalid{Reason: "index.json does not read as the archive index", Files: []string{FileIndex}, cause: err}
	}
	if index.Format != contract.ArchiveFormat1 || index.RunID != r.runID {
		return contract.Archive{}, &Invalid{Reason: "index.json is not format 1 of the same run", Files: []string{FileIndex}}
	}
	listed := map[string]bool{}
	for _, f := range index.Files {
		if listed[f.Path] {
			return contract.Archive{}, r.invalid("index.json lists a file twice", f.Path)
		}
		listed[f.Path] = true
		if !r.matches(f) {
			return contract.Archive{}, r.invalid("index.json lists a file the archive does not hold as listed", f.Path)
		}
	}
	for rel := range r.report {
		if !listed[DirReport+"/"+rel] {
			return contract.Archive{}, r.invalid("the archive holds a file index.json does not list", DirReport+"/"+rel)
		}
	}
	for rel := range r.schema {
		if !listed[rel] {
			return contract.Archive{}, r.invalid("the archive holds a file index.json does not list", rel)
		}
	}
	return index, nil
}

func (r *reader) matches(f contract.ArchiveFile) bool {
	if rel, ok := strings.CutPrefix(f.Path, DirReport+"/"); ok {
		size, held := r.report[rel]
		kind, known := KindOf(rel)
		return held && known && size == f.Bytes && kind.Name == f.Kind
	}
	size, held := r.schema[f.Path]
	return held && size == f.Bytes && f.Kind == contract.ArchiveKindSchema
}

func (r *reader) checkBundle() (contract.BundleMeta, error) {
	for _, rel := range []string{report.FileSummary, report.FileMeta} {
		if _, held := r.report[rel]; !held {
			return contract.BundleMeta{}, &Invalid{Reason: "the bundle has no " + rel, Files: []string{DirReport + "/" + rel}}
		}
	}
	takes := map[string]int{}
	for rel := range r.report {
		if take, ok := takeOf(rel); ok {
			takes[take]++
		}
	}
	for take, n := range takes {
		if n != 2 {
			return contract.BundleMeta{}, r.invalid("a wire take lacks one of its two files", DirReport+"/wire/"+take)
		}
	}
	raw, err := r.root.ReadFile(DirReport + "/" + report.FileMeta)
	if err != nil {
		return contract.BundleMeta{}, err
	}
	var meta contract.BundleMeta
	if err := strict(raw, &meta); err != nil {
		return contract.BundleMeta{}, &Invalid{Reason: "meta.json does not read as the bundle metadata", Files: []string{DirReport + "/" + report.FileMeta}, cause: err}
	}
	if meta.ID != r.runID || !slices.Contains(finalVerdicts, meta.Verdict) || meta.CreatedAt == "" {
		return contract.BundleMeta{}, &Invalid{Reason: "meta.json names another run, no final verdict or no creation time", Files: []string{DirReport + "/" + report.FileMeta}}
	}
	raw, err = r.root.ReadFile(DirReport + "/" + report.FileSummary)
	if err != nil {
		return contract.BundleMeta{}, err
	}
	var summary struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(raw, &summary); err != nil || summary.RunID != r.runID {
		return contract.BundleMeta{}, &Invalid{Reason: "summary.json does not name the same run", Files: []string{DirReport + "/" + report.FileSummary}}
	}
	return meta, nil
}
