package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/labels"
)

const (
	exportLimit   = 10 * time.Minute
	exportPartial = ".partial"
	exportMode    = 0o600
	problemLimit  = 1 << 20
)

var errExportBroken = errors.New("the archive did not arrive whole")

func (x *ctx) export(id, out string) (int, error) {
	target := out
	if target == "" {
		target = id + api.ExportSuffix
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(x.io.Cwd, target)
	}
	if _, err := os.Lstat(target); err == nil {
		return x.usage(labels.Text(labels.MsgExportExists, map[string]string{"file": target})), nil
	}
	c, cancel := context.WithTimeout(x, exportLimit)
	defer cancel()
	req, err := http.NewRequestWithContext(c, http.MethodGet, x.base.Scheme+"://"+x.base.Host+"/api/runs/"+id+"/export", nil)
	if err != nil {
		return 0, err
	}
	res, err := x.io.Client.Do(req)
	if err != nil {
		return 0, unreachable{err}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, err := io.ReadAll(io.LimitReader(res.Body, problemLimit))
		if err != nil {
			return 0, unreachable{err}
		}
		return x.refused(replyOf(res.StatusCode, raw)), nil
	}
	written, err := save(target, res.Body)
	if err != nil && !errors.Is(err, errExportBroken) {
		return 0, err
	}
	if err != nil {
		line := labels.Text(labels.MsgExportBroken, map[string]string{"file": target})
		return x.emit(ExitError, CodeCLIError, map[string]any{"id": id, "message": line}, []string{line}, true), nil
	}
	line := labels.Text(labels.MsgExported, map[string]string{"id": id, "bytes": strconv.FormatInt(written, 10), "file": target})
	return x.emit(ExitOK, CodeOK, map[string]any{"id": id, "file": target, "bytes": written}, []string{line}, false), nil
}

func save(target string, from io.Reader) (int64, error) {
	partial := target + exportPartial
	f, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, exportMode)
	if err != nil {
		return 0, err
	}
	written, err := io.Copy(f, from)
	if err != nil {
		err = errExportBroken
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(partial, target)
	}
	if err != nil {
		os.Remove(partial)
		return 0, err
	}
	return written, nil
}
