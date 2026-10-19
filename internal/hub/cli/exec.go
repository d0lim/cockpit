package cli

import (
	"net/http"

	"github.com/d0lim/cockpit/internal/hub/api"
)

var runnerExit = map[int]struct {
	exit int
	code string
}{
	0: {ExitOK, CodePassed},
	1: {ExitFailed, CodeFailed},
	2: {ExitError, CodeError},
}

func (x *ctx) exec(id string, a args) (int, error) {
	if len(a.rest) != 1 || a.rest[0] == "" {
		return x.usage("exec needs exactly one flow id: " + Cmd + " exec <id> -- <flow id>"), nil
	}
	r, err := x.call(http.MethodPost, "/api/runs/"+id+"/exec", map[string]any{"flow": a.rest[0], "submitter": x.submitter(a)}, requestLimit*20)
	if err != nil {
		return 0, err
	}
	if r.status != http.StatusOK {
		return x.refused(r), nil
	}
	view, err := decode[api.ExecView](r.body)
	if err != nil {
		return 0, err
	}
	result := struct {
		exit int
		code string
	}{ExitError, CodeError}
	if view.ExitCode != nil {
		if mapped, ok := runnerExit[*view.ExitCode]; ok {
			result = mapped
		}
	}
	return x.emit(result.exit, result.code, r.body, execLines(view), false), nil
}
