package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/labels"
	"github.com/d0lim/cockpit/internal/hub/pool"
)

const JSONSchemaVersion = 1

const (
	CodeOK             = "OK"
	CodeSubmitted      = "SUBMITTED"
	CodePassed         = "PASSED"
	CodeFailed         = "FAILED"
	CodeError          = "ERROR"
	CodeAborted        = "ABORTED"
	CodeUsage          = "USAGE"
	CodeHubUnreachable = "HUB_UNREACHABLE"
	CodeHubError       = "HUB_ERROR"
	CodeTimeout        = "TIMEOUT"
	CodeWaitingOnHuman = "WAITING_ON_HUMAN"
	CodeCLIError       = "CLI_ERROR"
)

func StableCodes() []string {
	own := []string{CodeOK, CodeSubmitted, CodePassed, CodeFailed, CodeError, CodeAborted, CodeUsage, CodeHubUnreachable, CodeHubError, CodeTimeout, CodeWaitingOnHuman, CodeCLIError}
	return slices.Concat(own, api.Codes)
}

var verdictExit = map[string]struct {
	exit int
	code string
}{
	"passed":  {ExitOK, CodePassed},
	"failed":  {ExitFailed, CodeFailed},
	"error":   {ExitError, CodeError},
	"aborted": {ExitConflict, CodeAborted},
}

type reply struct {
	status int
	body   map[string]any
}

func (x *ctx) emit(exit int, code string, data map[string]any, lines []string, toErr bool) int {
	if x.json {
		out := maps.Clone(data)
		if out == nil {
			out = map[string]any{}
		}
		out["schemaVersion"], out["code"] = JSONSchemaVersion, code
		line, err := json.Marshal(out)
		if err != nil {
			line = []byte(fmt.Sprintf(`{"schemaVersion":1,"code":%q}`, CodeCLIError))
		}
		fmt.Fprintf(x.io.Stdout, "%s\n", line)
		return exit
	}
	w := x.io.Stdout
	if toErr {
		w = x.io.Stderr
	}
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
	return exit
}

func (x *ctx) call(method, path string, body any, limit time.Duration) (reply, error) {
	origin := x.base.Scheme + "://" + x.base.Host
	var payload io.Reader
	if method == http.MethodPost {
		raw, err := json.Marshal(body)
		if err != nil {
			return reply{}, err
		}
		payload = bytes.NewReader(raw)
	}
	c, cancel := context.WithTimeout(x, limit)
	defer cancel()
	req, err := http.NewRequestWithContext(c, method, origin+path, payload)
	if err != nil {
		return reply{}, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set(api.HeaderCockpit, "1")
	}
	res, err := x.io.Client.Do(req)
	if err != nil {
		return reply{}, unreachable{err}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return reply{}, unreachable{err}
	}
	r := reply{status: res.StatusCode, body: map[string]any{}}
	if len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &r.body) != nil {
		r.body = map[string]any{"message": fmt.Sprintf("a response that is not a JSON object (HTTP %d)", res.StatusCode)}
	}
	return r, nil
}

func decode[T any](body map[string]any) (T, error) {
	var out T
	raw, err := json.Marshal(body)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func (x *ctx) refused(r reply) int {
	code := str(r.body["code"])
	if code == "" {
		code = CodeHubError
		if r.status == http.StatusForbidden {
			code = api.CodeForbidden
		}
	}
	message := str(r.body["message"])
	if message == "" {
		message = fmt.Sprintf("HTTP %d", r.status)
	}
	exit := ExitError
	switch {
	case r.status == http.StatusConflict:
		exit = ExitConflict
	case r.status >= 400 && r.status < 500:
		exit = ExitRejected
	}
	lines := []string{fmt.Sprintf("%s (%s)", message, code)}
	if files, ok := r.body["files"].([]any); ok {
		for _, f := range files {
			lines = append(lines, "  - "+str(f))
		}
	}
	if cands, err := decode[struct {
		Candidates []pool.Candidate `json:"candidates"`
	}](r.body); err == nil && len(cands.Candidates) > 0 {
		lines = append(lines, labels.Text(labels.MsgCandidates, nil))
		for _, c := range cands.Candidates {
			line := fmt.Sprintf("  - %s · up %s · %s", c.ID, orDash(c.LastUpAt), c.Submitter)
			if c.Keep {
				line += " · " + labels.Text(labels.MsgKeep, nil)
			}
			lines = append(lines, line)
		}
	}
	if run, ok := r.body["run"].(map[string]any); ok {
		lines = append(lines, labels.Text(labels.MsgNowState, map[string]string{"phase": str(run["phaseLabel"])}))
	}
	data := maps.Clone(r.body)
	data["status"] = r.status
	return x.emit(exit, code, data, lines, true)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
