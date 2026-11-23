package screen

import (
	"net/http"

	"github.com/d0lim/cockpit/internal/archive"
	"github.com/d0lim/cockpit/internal/hub/api"
)

const (
	importField   = "archive"
	maxImportForm = archive.ImportMaxBytes + 1<<20
)

func (s *Screen) importRun(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportForm)
	form, err := r.MultipartReader()
	if err != nil {
		s.answer(w, r, http.StatusBadRequest, problemResult(&api.Problem{Code: api.CodeBadRequest, Message: "the form cannot be read"}))
		return
	}
	for {
		part, err := form.NextPart()
		if err != nil {
			s.answer(w, r, http.StatusBadRequest, problemResult(&api.Problem{Code: api.CodeBadRequest, Message: "pick a bundle archive or the zip of an artifact"}))
			return
		}
		if part.FormName() != importField {
			part.Close()
			continue
		}
		got, err := s.d.Reader.Import(part)
		part.Close()
		if err != nil {
			s.fail(w, r, err)
			return
		}
		target := "/runs/" + got.ID
		if r.Header.Get("HX-Request") == "" {
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
		w.Header().Set("HX-Redirect", target)
		s.answer(w, r, http.StatusOK, importedResult(got))
		return
	}
}
