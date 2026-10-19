package dockerport

import (
	"archive/tar"
	"context"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var fileMutations = []porttest.Mutation{
	{"file byte limit", "return the file content without clip", "red: 500 bytes come back with truncated false", "TestFileOverTheByteLimitIsTruncatedAndFileWithinItIsNot"},
	{"symbolic links are not followed", "count link entries as files or copy again from the link target", "red: a link-only archive returns text or a second cp call is made", "TestSymbolicLinkEntriesAreNotFollowedAndRegularEntriesBesideThemAreRead"},
	{"tar entry limit", "remove the entries == maxEntries check", "red: all five files are read with truncated false", "TestArchivesWithMoreEntriesThanTheLimitAreTruncated"},
	{"paths are absolute and targets are not harness services", "drop the path check or resolve harness services", "red: a relative path or the edge container reaches docker cp", "TestRelativePathsAreBadArgsAndHarnessTargetsAreDeniedBeforeDockerCp"},
	{"file text passes the masker", "return raw file content", "red: the planted value shows in the file text", "TestFileOverTheByteLimitIsTruncatedAndFileWithinItIsNot"},
}

func cpReturning(archive []byte) func(context.Context, compose.Command) (compose.Result, error) {
	return func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		_, _ = cmd.Stdout.Write(archive)
		return compose.Result{}, nil
	}
}

func TestFileOverTheByteLimitIsTruncatedAndFileWithinItIsNot(t *testing.T) {
	f := &fakeDocker{handle: cpReturning(tarball(t, tarEntry{name: "audit.log", kind: tar.TypeReg, body: strings.Repeat("a", 500)}))}
	p, _ := newPort(t, f, Options{})
	out, err := p.File(context.Background(), FileRequest{Service: "app", Path: "/var/log/audit.log", MaxBytes: 100})
	if err != nil || len(out.Text) != 100 || !out.Truncated {
		t.Fatalf("over: %d %v %v", len(out.Text), out.Truncated, err)
	}
	out, err = p.File(context.Background(), FileRequest{Service: "app", Path: "/var/log/audit.log", MaxBytes: 500})
	if err != nil || len(out.Text) != 500 || out.Truncated {
		t.Fatalf("at the limit: %d %v %v", len(out.Text), out.Truncated, err)
	}
	f.handle = cpReturning(tarball(t, tarEntry{name: "env", kind: tar.TypeReg, body: "KEY=" + planted + "\n"}))
	out, err = p.File(context.Background(), FileRequest{Service: "app", Path: "/app/env"})
	if err != nil || strings.Contains(out.Text, planted) || out.Text != "KEY=****\n" {
		t.Fatalf("masked: %+v %v", out, err)
	}
	cp := f.commands("cp")
	if len(cp) != 3 || cp[0][1] != "c-app:/var/log/audit.log" || cp[0][2] != "-" {
		t.Fatalf("%v", cp)
	}
}

func TestSymbolicLinkEntriesAreNotFollowedAndRegularEntriesBesideThemAreRead(t *testing.T) {
	f := &fakeDocker{handle: cpReturning(tarball(t, tarEntry{name: "current", kind: tar.TypeSymlink, link: "/etc/shadow"}))}
	p, _ := newPort(t, f, Options{})
	_, err := p.File(context.Background(), FileRequest{Service: "app", Path: "/app/current"})
	if codeOf(err) != engine.PortDenied || !strings.Contains(err.Error(), "links are not followed") {
		t.Fatalf("%v", err)
	}
	f.handle = cpReturning(tarball(t,
		tarEntry{name: "logs/", kind: tar.TypeDir},
		tarEntry{name: "logs/latest", kind: tar.TypeSymlink, link: "/etc/shadow"},
		tarEntry{name: "logs/hard", kind: tar.TypeLink, link: "logs/app.log"},
		tarEntry{name: "logs/app.log", kind: tar.TypeReg, body: "app line\n"},
	))
	out, err := p.File(context.Background(), FileRequest{Service: "app", Path: "/app/logs"})
	if err != nil || out.Text != "app line\n" || out.Truncated {
		t.Fatalf("%+v %v", out, err)
	}
	if cp := f.commands("cp"); len(cp) != 2 {
		t.Fatalf("links caused extra copies: %v", cp)
	}
}

func TestArchivesWithMoreEntriesThanTheLimitAreTruncated(t *testing.T) {
	var entries []tarEntry
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		entries = append(entries, tarEntry{name: n, kind: tar.TypeReg, body: n})
	}
	f := &fakeDocker{handle: cpReturning(tarball(t, entries...))}
	p, _ := newPort(t, f, Options{MaxEntries: 3})
	out, err := p.File(context.Background(), FileRequest{Service: "app", Path: "/data"})
	if err != nil || out.Text != "abc" || !out.Truncated {
		t.Fatalf("%+v %v", out, err)
	}
	q, _ := newPort(t, f, Options{MaxEntries: 5})
	out, err = q.File(context.Background(), FileRequest{Service: "app", Path: "/data"})
	if err != nil || out.Text != "abcde" || out.Truncated {
		t.Fatalf("at the limit: %+v %v", out, err)
	}
}

func TestRelativePathsAreBadArgsAndHarnessTargetsAreDeniedBeforeDockerCp(t *testing.T) {
	f := &fakeDocker{handle: cpReturning(tarball(t, tarEntry{name: "x", kind: tar.TypeReg, body: "x"}))}
	p, _ := newPort(t, f, Options{})
	bad := []FileRequest{{Service: "app", Path: "etc/passwd"}, {Service: "app", Path: ""}, {Service: "app", Path: "/a\x00b"}}
	for _, r := range bad {
		if _, err := p.File(context.Background(), r); codeOf(err) != engine.PortBadArgs {
			t.Errorf("%q: %v", r.Path, err)
		}
	}
	for _, svc := range []string{"edge", "probe", "toolbox", "intruder"} {
		if _, err := p.File(context.Background(), FileRequest{Service: svc, Path: "/etc/hosts"}); codeOf(err) != engine.PortDenied {
			t.Errorf("%s: %v", svc, err)
		}
	}
	if cp := f.commands("cp"); len(cp) != 0 {
		t.Fatalf("docker cp ran: %v (%d bad paths)", cp, len(bad))
	}
	if out, err := p.File(context.Background(), FileRequest{Service: "app", Path: "/etc/hosts"}); err != nil || out.Text != "x" {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestFileMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "file_test.go", fileMutations)
}
