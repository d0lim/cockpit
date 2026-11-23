package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/d0lim/cockpit/internal/archive"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/version"
)

const (
	ExportMedia  = "application/gzip"
	ExportSuffix = ".tar.gz"
)

func (a *api) export(w http.ResponseWriter, r *http.Request) error {
	v, err := a.runOf(r)
	if err != nil {
		return err
	}
	dir, err := a.runDir(v.ID)
	if err != nil {
		return err
	}
	missing := notFound("the run ended without a report bundle")
	switch reportView(dir, v.Meta).Status {
	case ReportPending:
		return &Problem{Status: http.StatusConflict, Code: CodeNotAvailable, Message: "the run has not written its report yet"}
	case ReportRedactionFailed:
		return &Problem{Status: http.StatusConflict, Code: CodeReadRefused, Message: report.RefusedRedaction, Files: v.Meta.Redaction.Files}
	case ReportMetaOnly:
		return missing
	}
	if a.d.Mask == nil {
		return &Problem{Status: http.StatusConflict, Code: CodeNotAvailable, Message: "no run masker: the bundle archive is not served"}
	}
	m := a.d.Mask(v.ID)
	spool, err := archive.OpenSpool()
	if err != nil {
		return err
	}
	defer spool.Close()
	_, err = archive.Write(spool, archive.Input{RunDir: dir, RunID: v.ID, Mask: m, Now: time.Now(), Version: version.String()})
	var refusal *archive.Refusal
	switch {
	case errors.As(err, &refusal):
		return &Problem{Status: http.StatusConflict, Code: CodeReadRefused, Message: refusal.Error(), Files: refusal.Files}
	case errors.Is(err, archive.ErrNoBundle):
		return missing
	case err != nil:
		return &Problem{Status: http.StatusInternalServerError, Code: CodeInternal, Message: m.Mask(err.Error())}
	}
	size, err := spool.Rewind()
	if err != nil {
		return err
	}
	h := w.Header()
	h.Set("Content-Type", ExportMedia)
	h.Set("Content-Disposition", `attachment; filename="`+v.ID+ExportSuffix+`"`)
	h.Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	if _, err := io.CopyN(w, spool, size); err != nil {
		a.log().Debug("response write failed", "error", err)
	}
	return nil
}
