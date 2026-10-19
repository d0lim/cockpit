package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/hub/labels"
	"github.com/d0lim/cockpit/internal/hub/lock"
)

const (
	ExitOK          = 0
	ExitFailed      = 1
	ExitError       = 2
	ExitRejected    = 3
	ExitUnreachable = 4
	ExitConflict    = 5
	ExitTimeout     = 6
	ExitHuman       = 7
)

const (
	Cmd           = "cockpit runs"
	EnvURL        = "COCKPIT_URL"
	EnvSubmitter  = "COCKPIT_SUBMITTER"
	cliSubmitter  = "cli"
	requestLimit  = 30 * time.Second
	defaultPoll   = 2 * time.Second
	defaultReconn = 60 * time.Second
)

var DefaultURL = "http://127.0.0.1:" + strconv.Itoa(lock.DefaultPort)

var (
	booleanFlags = map[string]bool{"wait": true, "new": true, "json": true, "force": true}
	valueFlags   = map[string]bool{"commit": true, "suite": true, "submitter": true, "timeout": true}
	urlPattern   = regexp.MustCompile(`^https?://[^/]+`)
)

var usageLines = []string{
	"usage: " + Cmd + " <command> ...",
	"  submit [--commit <ref>] [--suite <name>] [--submitter <label>] [--new] [--wait] [--timeout <seconds>] [--json]",
	"  list · get|show <id> · report <id> · wait <id> [--timeout <seconds>]",
	"  keep|unkeep|stop|resume|destroy|abort <id> [--force] · exec <id> -- <flow id>",
	"The hub address is COCKPIT_URL (default " + DefaultURL + "); the submitter label is --submitter, then COCKPIT_SUBMITTER, then cli.",
}

type IO struct {
	Getenv    func(string) string
	Cwd       string
	Stdout    io.Writer
	Stderr    io.Writer
	Poll      time.Duration
	Reconnect time.Duration
	Client    *http.Client
	Toplevel  func(ctx context.Context, cwd string) (string, error)
}

type args struct {
	sub        string
	positional []string
	flags      map[string]string
	rest       []string
}

type ctx struct {
	context.Context
	io   IO
	base *url.URL
	json bool
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

type unreachable struct{ err error }

func (e unreachable) Error() string { return e.err.Error() }

func parse(argv []string) (args, error) {
	a := args{flags: map[string]string{}}
	if len(argv) == 0 {
		return a, nil
	}
	a.sub = argv[0]
	tail := argv[1:]
	for i := 0; i < len(tail); i++ {
		arg := tail[i]
		if arg == "--" {
			a.rest = tail[i+1:]
			break
		}
		if !strings.HasPrefix(arg, "--") {
			a.positional = append(a.positional, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg[2:], "=")
		switch {
		case booleanFlags[name] && !hasValue:
			a.flags[name] = "true"
		case valueFlags[name]:
			if !hasValue {
				i++
				if i < len(tail) {
					value = tail[i]
				}
			}
			if value == "" {
				return a, usageError{"--" + name + " has no value"}
			}
			a.flags[name] = value
		default:
			return a, usageError{"unknown option: " + arg}
		}
	}
	return a, nil
}

func Main(c context.Context, argv []string, io IO) int {
	if io.Poll <= 0 {
		io.Poll = defaultPoll
	}
	if io.Reconnect <= 0 {
		io.Reconnect = defaultReconn
	}
	if io.Client == nil {
		io.Client = &http.Client{}
	}
	if io.Toplevel == nil {
		io.Toplevel = gitToplevel
	}
	words := argv
	options := words
	if i := indexOf(words, "--"); i >= 0 {
		options = words[:i]
	}
	raw := io.Getenv(EnvURL)
	if raw == "" {
		raw = DefaultURL
	}
	x := &ctx{Context: c, io: io, json: indexOf(options, "--json") >= 0}
	base, err := url.Parse(raw)
	if err != nil || !urlPattern.MatchString(raw) {
		return x.usage(EnvURL + " is not an http address: " + raw)
	}
	x.base = base
	code, err := x.dispatch(words)
	var ue usageError
	var un unreachable
	switch {
	case errors.As(err, &ue):
		return x.usage(ue.msg)
	case errors.As(err, &un):
		return x.unreachable("")
	case err != nil:
		return x.emit(ExitError, CodeCLIError, map[string]any{"message": err.Error()}, []string{labels.Text(labels.MsgUnexpected, map[string]string{"error": err.Error()})}, true)
	}
	return code
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func (x *ctx) dispatch(words []string) (int, error) {
	if len(words) == 0 || words[0] != "runs" {
		return 0, usageError{"the first word is runs"}
	}
	a, err := parse(words[1:])
	if err != nil {
		return 0, err
	}
	switch a.sub {
	case "submit":
		return x.submit(a)
	case "list":
		if len(a.positional) > 0 {
			return 0, usageError{"extra arguments: " + strings.Join(a.positional, " ")}
		}
		return x.list()
	case "get", "show":
		return withID(a, x.show)
	case "report":
		return withID(a, x.report)
	case "wait":
		timeout, err := timeoutOf(a)
		if err != nil {
			return 0, err
		}
		return withID(a, func(id string) (int, error) { return x.wait(id, timeout, false) })
	case "exec":
		return withID(a, func(id string) (int, error) { return x.exec(id, a) })
	case "keep", "unkeep", "stop", "resume", "destroy", "abort":
		return withID(a, func(id string) (int, error) { return x.operate(a, id) })
	case "":
		return 0, usageError{"no command"}
	}
	return 0, usageError{"unknown command: " + a.sub}
}

func timeoutOf(a args) (time.Duration, error) {
	raw, ok := a.flags["timeout"]
	if !ok {
		return 0, nil
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds <= 0 {
		return 0, usageError{"--timeout is a number of seconds above 0: " + raw}
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func (x *ctx) submitter(a args) string {
	if s := a.flags["submitter"]; s != "" {
		return s
	}
	if s := x.io.Getenv(EnvSubmitter); s != "" {
		return s
	}
	return cliSubmitter
}

func (x *ctx) usage(message string) int {
	return x.emit(ExitRejected, CodeUsage, map[string]any{"message": message}, append([]string{message}, usageLines...), true)
}

func (x *ctx) unreachable(waiting string) int {
	origin := x.base.Scheme + "://" + x.base.Host
	start := "cockpit hub"
	if port := x.base.Port(); port != "" && port != strconv.Itoa(lock.DefaultPort) {
		start = fmt.Sprintf("%s=%s cockpit hub", lock.EnvPort, port)
	}
	lines := []string{labels.Text(labels.MsgHubUnreachable, map[string]string{"url": origin}), labels.Text(labels.MsgHubStart, map[string]string{"start": start})}
	data := map[string]any{"url": origin}
	if waiting != "" {
		lines = append(lines, labels.Text(labels.MsgHubWaitAgain, map[string]string{"cmd": Cmd, "id": waiting}))
		data["id"] = waiting
	}
	return x.emit(ExitUnreachable, CodeHubUnreachable, data, lines, true)
}
