package recorder

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/wire"
)

func okUpstream() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, r.URL.Path) })
}

func TestATakeWaitsUntilNoCallHasStartedForTheQuietTime(t *testing.T) {
	r := startRig(t, okUpstream(), func(o *Options) { o.Limits.Quiet = 400 * time.Millisecond })
	r.get("/first")
	started := time.Now()
	got := r.take(InitialTake)
	if waited := time.Since(started); waited < 300*time.Millisecond {
		t.Fatalf("the take returned after %v", waited)
	}
	if len(got.calls) != 1 || got.end.Calls != 1 {
		t.Fatalf("take %+v", got)
	}
}

func TestATakeWithNoCallsReturnsOnlyTheEndLineAtOnce(t *testing.T) {
	before := time.Now()
	r := startRig(t, okUpstream(), func(o *Options) { o.Limits.Quiet = time.Minute })
	started := time.Now()
	got := r.take(InitialTake)
	since := got.end.Since
	got.end.Since = time.Time{}
	if len(got.calls) != 0 || got.end != (wire.End{}) || time.Since(started) > 2*time.Second {
		t.Fatalf("take %+v after %v", got, time.Since(started))
	}
	if since.Before(before) || since.After(started) {
		t.Fatalf("the end line says the recorder started at %v, between %v and %v", since, before, started)
	}
}

func TestACallStillOpenAtTheLimitComesOutIncompleteWithoutAFinishTime(t *testing.T) {
	release := make(chan struct{})
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.Copy(io.Discard, req.Body)
		w.WriteHeader(http.StatusAccepted)
		w.(http.Flusher).Flush()
		<-release
	}), func(o *Options) { o.Limits.MaxWait = 150 * time.Millisecond })
	defer close(release)
	res, err := r.client.Post(r.url("/slow"), "text/plain", strings.NewReader("asked"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	c := r.onlyCall()
	if c.Outcome != wire.OutcomeIncomplete || !c.Finished.IsZero() || c.Status != http.StatusAccepted {
		t.Fatalf("call %+v", c)
	}
	if c.Request.Body.Text != "asked" {
		t.Fatalf("the finished request body is missing: %+v", c.Request)
	}
}

func TestARepeatedTakeReturnsTheSameBytesAndCallsAfterItPassUnrecorded(t *testing.T) {
	r := startRig(t, okUpstream(), nil)
	r.get("/one")
	first, err := r.control("take " + InitialTake)
	if err != nil {
		t.Fatal(err)
	}
	r.get("/after-the-take")
	second, err := r.control("take " + InitialTake)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !strings.Contains(first, "/one") {
		t.Fatalf("first %q second %q", first, second)
	}
	if strings.Contains(second, "/after-the-take") {
		t.Fatalf("a call after the take entered the record: %q", second)
	}
}

func TestArmClearsTheRecordAndRecordsUnderTheNewName(t *testing.T) {
	r := startRig(t, okUpstream(), nil)
	r.get("/suite-call")
	if _, err := r.control("arm exec-1"); err != nil {
		t.Fatal(err)
	}
	r.get("/exec-call")
	got := r.take("exec-1")
	if len(got.calls) != 1 || got.calls[0].Path != "/exec-call" || got.calls[0].Seq != 1 || got.end.Calls != 1 {
		t.Fatalf("take %+v", got)
	}
	if _, err := r.control("take " + InitialTake); err == nil || !strings.Contains(err.Error(), "exec-1") {
		t.Fatalf("the old take after arm: %v", err)
	}
}

func TestACallFromBeforeAnArmDoesNotEnterTheNewTake(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
	}), nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.get("/old")
	}()
	<-entered
	if _, err := r.control("arm exec-1"); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done
	if got := r.take("exec-1"); len(got.calls) != 0 || got.end.Calls != 0 {
		t.Fatalf("take %+v", got)
	}
}

func TestATakeOfAnotherNameFailsAndWritesNoLines(t *testing.T) {
	r := startRig(t, okUpstream(), nil)
	r.get("/one")
	out, err := r.control("take other")
	if err == nil || !strings.Contains(err.Error(), `"other"`) || !strings.Contains(err.Error(), InitialTake) || out != "" {
		t.Fatalf("out %q err %v", out, err)
	}
	if got := r.take(InitialTake); len(got.calls) != 1 {
		t.Fatalf("the failed take changed the record: %+v", got)
	}
}

func TestCallsComeOutInStartOrderWhateverOrderTheyFinished(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/first" {
			entered <- struct{}{}
			<-release
		}
	}), nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.get("/first")
	}()
	<-entered
	r.get("/second")
	close(release)
	<-done
	got := r.take(InitialTake)
	if len(got.calls) != 2 || got.calls[0].Path != "/first" || got.calls[0].Seq != 1 || got.calls[1].Path != "/second" || got.calls[1].Seq != 2 {
		t.Fatalf("calls %+v", got.calls)
	}
}

func TestPingAnswersWithoutEnteringTheRecordAndAnUnknownRequestFails(t *testing.T) {
	r := startRig(t, okUpstream(), nil)
	if out, err := r.control("ping"); err != nil || out != "" {
		t.Fatalf("ping out %q err %v", out, err)
	}
	if _, err := r.control("sing"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("an unknown request: %v", err)
	}
	if got := r.take(InitialTake); len(got.calls) != 0 {
		t.Fatalf("calls %+v", got.calls)
	}
}
