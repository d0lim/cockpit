package api

import (
	"errors"
	"mime"
	"net/http"
	"os"

	"github.com/d0lim/cockpit/internal/archive"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/mask"
)

type Imported struct {
	ID      string                `json:"id"`
	Verdict contract.FinalVerdict `json:"verdict"`
	Files   int                   `json:"files"`
	Run     RunItem               `json:"run"`
}

func (a *api) importRun(w http.ResponseWriter, r *http.Request) error {
	if media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); media != ExportMedia {
		return &Problem{Status: http.StatusUnsupportedMediaType, Code: CodeBadRequest, Message: "an import body is a bundle archive sent as " + ExportMedia}
	}
	m, err := mask.New(mask.Options{Mode: mask.Hub})
	if err != nil {
		return err
	}
	stage, err := a.d.Store.ImportStage()
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	got, err := archive.Unpack(http.MaxBytesReader(w, r.Body, archive.ImportMaxBytes), stage, m)
	if err != nil {
		return importProblem(err, m)
	}
	meta := store.ImportedMeta(got.Meta)
	err = a.d.Store.Adopt(stage, meta)
	if errors.Is(err, store.ErrRunExists) {
		return &Problem{Status: http.StatusConflict, Code: string(ticker.CodeConflictState), Message: got.RunID + ": " + err.Error()}
	}
	if err != nil {
		return &Problem{Status: http.StatusInternalServerError, Code: CodeInternal, Message: m.Mask(err.Error())}
	}
	a.changed()
	writeJSON(a.log(), w, http.StatusCreated, Imported{ID: got.RunID, Verdict: got.Meta.Verdict, Files: got.Files, Run: MetaItem(meta)})
	return nil
}

func importProblem(err error, m *mask.Masker) *Problem {
	var tooBig *http.MaxBytesError
	var invalid *archive.Invalid
	var unmasked *archive.Unmasked
	switch {
	case errors.As(err, &tooBig), errors.Is(err, archive.ErrTooLarge):
		return &Problem{Status: http.StatusRequestEntityTooLarge, Code: CodeBadRequest, Message: m.Mask(err.Error())}
	case errors.As(err, &unmasked):
		return &Problem{Status: http.StatusConflict, Code: CodeReadRefused, Message: unmasked.Error(), Files: unmasked.Files}
	case errors.As(err, &invalid):
		return &Problem{Status: http.StatusBadRequest, Code: CodeBadRequest, Message: invalid.Error(), Files: invalid.Files}
	}
	return &Problem{Status: http.StatusInternalServerError, Code: CodeInternal, Message: m.Mask(err.Error())}
}
