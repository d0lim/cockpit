package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/labels"
)

var zipMagic = []byte("PK\x03\x04")

func (x *ctx) importArchive(a args) (int, error) {
	if len(a.positional) != 1 {
		return 0, usageError{"import needs one archive file"}
	}
	file := a.positional[0]
	if !filepath.IsAbs(file) {
		file = filepath.Join(x.io.Cwd, file)
	}
	f, err := os.Open(file)
	if err != nil {
		return x.usage(labels.Text(labels.MsgImportUnreadable, map[string]string{"file": file})), nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return x.usage(labels.Text(labels.MsgImportUnreadable, map[string]string{"file": file})), nil
	}
	media := api.ExportMedia
	head := make([]byte, len(zipMagic))
	if n, _ := io.ReadFull(f, head); bytes.Equal(head[:n], zipMagic) {
		media = api.ZipMedia
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	c, cancel := context.WithTimeout(x, exportLimit)
	defer cancel()
	origin := x.base.Scheme + "://" + x.base.Host
	req, err := http.NewRequestWithContext(c, http.MethodPost, origin+"/api/runs/import", f)
	if err != nil {
		return 0, err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Content-Type", media)
	req.Header.Set("Origin", origin)
	req.Header.Set("X-Cockpit", "1")
	res, err := x.io.Client.Do(req)
	if err != nil {
		return 0, unreachable{err}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, problemLimit))
	if err != nil {
		return 0, unreachable{err}
	}
	if res.StatusCode != http.StatusCreated {
		return x.refused(replyOf(res.StatusCode, raw)), nil
	}
	var got api.Imported
	if err := json.Unmarshal(raw, &got); err != nil {
		return 0, err
	}
	url := origin + "/runs/" + got.ID
	line := labels.Text(labels.MsgImported, map[string]string{"id": got.ID, "verdict": string(got.Verdict), "files": strconv.Itoa(got.Files), "url": url})
	return x.emit(ExitOK, CodeOK, map[string]any{"id": got.ID, "verdict": got.Verdict, "files": got.Files, "url": url}, []string{line}, false), nil
}
