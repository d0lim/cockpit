package compose

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const psArray = `[{"Service":"app","Name":"r-toy-app-1","ID":"aaa","State":"running","Health":"","ExitCode":0,"Labels":"com.docker.compose.project=r-toy"},
{"Service":"runner","Name":"r-toy-runner-run-1","ID":"bbb","State":"exited","Health":"","ExitCode":1,"Labels":"com.docker.compose.oneoff=True,com.docker.compose.project=r-toy"},
{"Service":"db","Name":"r-toy-db-1","ID":"ccc","State":"running","Health":"healthy","ExitCode":0,"Labels":""}]`

func TestComposePsParsesTheSameFromAnArrayAndFromJSONLines(t *testing.T) {
	fromArray, err := ParsePs([]byte("  \n" + psArray))
	if err != nil {
		t.Fatal(err)
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(psArray, "["), "]")
	jsonLines := strings.ReplaceAll(inner, "},\n{", "}\n{") + "\n"
	fromLines, err := ParsePs([]byte(jsonLines))
	if err != nil {
		t.Fatal(err)
	}
	if len(fromArray) != 3 || !reflect.DeepEqual(fromArray, fromLines) {
		t.Fatalf("array %+v\nlines %+v", fromArray, fromLines)
	}
	if !fromArray[1].OneOff() || fromArray[0].OneOff() || fromArray[2].Health != "healthy" {
		t.Fatalf("rows %+v", fromArray)
	}
	for _, empty := range []string{"", "  \n", "[]"} {
		rows, err := ParsePs([]byte(empty))
		if err != nil || len(rows) != 0 {
			t.Errorf("%q: %v %v", empty, rows, err)
		}
	}
	for _, bad := range []string{"[{", "{\"Service\":1}", "{} x"} {
		if _, err := ParsePs([]byte(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func versionDocker(compose string) *recordingDocker {
	return &recordingDocker{reply: func(args []string) Result {
		if args[0] == "version" {
			return Result{Stdout: []byte("29.8.1\n")}
		}
		return Result{Stdout: []byte(compose + "\n")}
	}}
}

type recordingDocker struct {
	calls [][]string
	reply func(args []string) Result
}

func (r *recordingDocker) Run(ctx context.Context, cmd Command) (Result, error) {
	r.calls = append(r.calls, cmd.Args)
	return r.reply(cmd.Args), nil
}

func TestComposeOlderThan2Dot24IsRefusedAndNewerReleasesPass(t *testing.T) {
	for _, ok := range []string{"2.24.0", "v2.29.7", "5.5.1", "2.40.3-desktop.1"} {
		if err := CheckVersions(context.Background(), versionDocker(ok)); err != nil {
			t.Errorf("%s was refused: %v", ok, err)
		}
	}
	for _, old := range []string{"2.23.3", "v1.29.2", "garbage"} {
		if err := CheckVersions(context.Background(), versionDocker(old)); err == nil {
			t.Errorf("%s passed", old)
		}
	}
}

func TestProjectCallsNameTheProjectAndKeepTheComposeHygieneFlags(t *testing.T) {
	d := &recordingDocker{reply: func([]string) Result { return Result{Stdout: []byte("[]")} }}
	l := testLayout(t)
	args, err := ComposeArgs("r-toy", l, toyDeclaration(t))
	if err != nil {
		t.Fatal(err)
	}
	p := Project{Docker: d, Name: "r-toy", Args: args, Mask: func(s string) string { return s }}
	ctx := context.Background()
	if err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Ps(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Down(ctx); err != nil {
		t.Fatal(err)
	}
	for _, call := range d.calls {
		if call[0] != "compose" {
			continue
		}
		if !slices.Contains(call, "--ansi") || call[slices.Index(call, "-p")+1] != "r-toy" {
			t.Errorf("call without the project name or --ansi never: %q", call)
		}
	}
	up := d.calls[0]
	if !slices.Contains(up, "--project-directory") || !slices.Equal(up[len(up)-3:], []string{"up", "-d", "--no-build"}) {
		t.Fatalf("up %q", up)
	}
	down := d.calls[len(d.calls)-1]
	if !slices.Equal(down[len(down)-7:], []string{"--profile", "*", "down", "-v", "--remove-orphans", "--rmi", "local"}) {
		t.Fatalf("down %q", down)
	}
}
