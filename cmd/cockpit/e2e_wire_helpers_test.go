package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
)

const wireRunLimit = "1200"

type wireRun struct {
	e  *e2e
	id string
}

func (e *e2e) submit(args ...string) wireRun {
	e.t.Helper()
	code, out, errOut := e.cockpit(e.repo, append([]string{"runs", "submit", "--json"}, args...)...)
	var answer struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &answer); err != nil || code != 0 || answer.ID == "" {
		e.t.Fatalf("submit %v: exit %d %v\n%s%s", args, code, err, out, errOut)
	}
	return wireRun{e: e, id: answer.ID}
}

func (w wireRun) dir() string { return filepath.Join(w.e.home, "runs", w.id) }

func (w wireRun) wait() string {
	w.e.t.Helper()
	_, out, errOut := w.e.cockpit(w.e.repo, "runs", "wait", w.id, "--json", "--timeout", wireRunLimit)
	var answer struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &answer); err != nil {
		w.e.t.Fatalf("wait: %v\n%s%s", err, out, errOut)
	}
	return answer.Verdict
}

func (w wireRun) ok(op string, args ...string) {
	w.e.t.Helper()
	if code, out, errOut := w.e.cockpit(w.e.repo, append([]string{"runs", op, w.id}, args...)...); code != 0 {
		w.e.t.Fatalf("runs %s: exit %d\n%s%s", op, code, out, errOut)
	}
}

func (w wireRun) execFlow(flow string) string {
	w.e.t.Helper()
	before := map[string]bool{}
	for _, take := range w.view("").Takes {
		before[take] = true
	}
	if code, out, errOut := w.e.cockpit(w.e.repo, "runs", "exec", w.id, "--json", "--", flow); code != 0 {
		w.e.t.Fatalf("runs exec: exit %d\n%s%s", code, out, errOut)
	}
	for _, take := range w.view("").Takes {
		if !before[take] {
			return take
		}
	}
	w.e.t.Fatal("the exec left no take")
	return ""
}

func (w wireRun) get(path string) (int, string) {
	w.e.t.Helper()
	res, err := http.Get(w.e.url + path)
	if err != nil {
		w.e.t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		w.e.t.Fatal(err)
	}
	return res.StatusCode, string(body)
}

func (w wireRun) getJSON(path string, into any) string {
	w.e.t.Helper()
	status, body := w.get(path)
	if status != http.StatusOK {
		w.e.t.Fatalf("GET %s: %d %s", path, status, body)
	}
	if err := json.Unmarshal([]byte(body), into); err != nil {
		w.e.t.Fatalf("GET %s: %v", path, err)
	}
	return body
}

func (w wireRun) view(query string) api.WireView {
	w.e.t.Helper()
	var v api.WireView
	sep := "?"
	if strings.Contains(query, "?") {
		sep = "&"
	}
	w.getJSON("/api/runs/"+w.id+"/wire"+query+sep+"limit=500", &v)
	return v
}

func (w wireRun) call(take string, seq int64) api.WireCallView {
	w.e.t.Helper()
	var v api.WireCallView
	w.getJSON("/api/runs/"+w.id+"/wire/"+itoa(seq)+"?take="+take, &v)
	return v
}

func (w wireRun) summary() contract.Summary {
	w.e.t.Helper()
	var s contract.Summary
	readJSONFile(w.e.t, filepath.Join(w.dir(), "report", "summary.json"), &s)
	return s
}

func readJSONFile(t interface {
	Helper()
	Fatal(...any)
}, path string, into any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatal(err)
	}
}

func (w wireRun) watchReads() func() int {
	stop := make(chan struct{})
	done := make(chan struct{})
	var mu sync.Mutex
	reads := 0
	var failures []string
	read := func(client *http.Client, path string) {
		res, err := client.Get(w.e.url + path)
		if err != nil {
			return
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		mu.Lock()
		defer mu.Unlock()
		reads++
		if res.StatusCode != http.StatusOK {
			failures = append(failures, path+": "+res.Status+" "+string(body))
		}
	}
	go func() {
		defer close(done)
		client := &http.Client{Timeout: 10 * time.Second}
		for {
			select {
			case <-stop:
				return
			case <-time.After(15 * time.Millisecond):
			}
			read(client, "/api/runs/"+w.id+"/wire?limit=1")
			read(client, "/runs/"+w.id+"/wire")
		}
	}()
	return func() int {
		close(stop)
		<-done
		mu.Lock()
		defer mu.Unlock()
		for i, f := range failures {
			if i == 5 {
				break
			}
			w.e.t.Errorf("a read while the run moved failed: %s", f)
		}
		if reads == 0 {
			w.e.t.Error("no read of the record was made while the run moved")
		}
		return reads
	}
}

func (w wireRun) awaitLog(service, text string) {
	w.e.t.Helper()
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if strings.Contains(w.e.logs(w.id, service), text) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	w.e.t.Fatalf("%s never logged %s", service, text)
}

func (e *e2e) container(project, service string) string {
	e.t.Helper()
	out, err := exec.Command("docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+project, "--filter", "label=com.docker.compose.service="+service).Output()
	if err != nil {
		e.t.Fatal(err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func (e *e2e) logs(project, service string) string {
	e.t.Helper()
	id := e.container(project, service)
	if id == "" {
		return ""
	}
	out, err := exec.Command("docker", "logs", id).CombinedOutput()
	if err != nil {
		e.t.Fatalf("docker logs %s: %v", service, err)
	}
	return string(out)
}

func (e *e2e) containerEnv(project, service, name string) string {
	e.t.Helper()
	id := e.container(project, service)
	if id == "" {
		e.t.Fatalf("no %s container in %s", service, project)
	}
	out, err := exec.Command("docker", "inspect", "--format", "{{json .Config.Env}}", id).Output()
	if err != nil {
		e.t.Fatal(err)
	}
	var env []string
	if err := json.Unmarshal(out, &env); err != nil {
		e.t.Fatal(err)
	}
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			return v
		}
	}
	return ""
}
