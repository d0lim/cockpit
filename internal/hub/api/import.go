package api

import (
	"errors"
	"io"
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
	if media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); media != ExportMedia && media != ZipMedia {
		return &Problem{Status: http.StatusUnsupportedMediaType, Code: CodeBadRequest, Message: "an import body is a bundle archive sent as " + ExportMedia + ", or an artifact zip that holds one sent as " + ZipMedia}
	}
	got, err := a.importArchive(http.MaxBytesReader(w, r.Body, archive.ImportMaxBytes))
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusCreated, got)
	return nil
}

func (a *api) importArchive(body io.Reader) (Imported, error) {
	m, err := mask.New(mask.Options{Mode: mask.Hub})
	if err != nil {
		return Imported{}, err
	}
	stage, err := a.d.Store.ImportStage()
	if err != nil {
		return Imported{}, err
	}
	defer os.RemoveAll(stage)
	got, err := archive.UnpackFile(body, stage, m)
	if err != nil {
		return Imported{}, importProblem(err, m)
	}
	meta := store.ImportedMeta(got.Meta)
	err = a.d.Store.Adopt(stage, meta)
	if errors.Is(err, store.ErrRunExists) {
		return Imported{}, &Problem{Status: http.StatusConflict, Code: string(ticker.CodeConflictState), Message: got.RunID + ": " + err.Error()}
	}
	if err != nil {
		return Imported{}, &Problem{Status: http.StatusInternalServerError, Code: CodeInternal, Message: m.Mask(err.Error())}
	}
	a.changed()
	return Imported{ID: got.RunID, Verdict: got.Meta.Verdict, Files: got.Files, Run: MetaItem(meta)}, nil
}

func (r *Reader) Import(body io.Reader) (Imported, error) { return r.a.importArchive(body) }

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
