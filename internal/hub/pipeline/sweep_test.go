package pipeline

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
)

var sweepMutations = []porttest.Mutation{
	{"the sql port scrubs the root secret and the reader password from its errors", "pass p.mask instead of scrubber(e.secrets, p.mask) to failure in Port.ensure", "red: the provisioned reader password is in not-proven.md in five encodings", "TestEveryPlantedCredentialIsAbsentInEveryEncodingFromTheBundleTheHubHomeAndTheHubLogAfterReportAndDestroy"},
	{"the live home scan tolerates a file renamed away during the walk", "fail the walk on fs.ErrNotExist in leakScan.tree", "red: the scan stops with no such file for the vanished file", "TestTheLeakScanSkipsAFileThatVanishesDuringTheWalkAndStillReadsTheRest"},
	{"collector state passes the run redactor before it is stored", "store the state key and value without mask", "red: the collector state value is in signals/kv/sweep.json", "TestEveryPlantedCredentialIsAbsentInEveryEncodingFromTheBundleTheHubHomeAndTheHubLogAfterReportAndDestroy"},
}

func TestSweepMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "sweep_test.go", sweepMutations)
}

type canary struct {
	slot  string
	value string
}

var sweepCanaries = []canary{
	{"secret source and env output .env", "PLANTED-dotenv-Q7+/<&w1x"},
	{"sql root secret", "PLANTED-rootpw-R8+/<&k2x"},
	{"private env value", "PLANTED-private-P9+/<&m3x"},
	{"runner output line", "PLANTED-runner-N4+/<&z5x"},
	{"runner step message", "PLANTED-stepmsg-S6+/<&y7x"},
	{"signal attr key", "PLANTED-attrkey-K1+/<&q6x"},
	{"signal attr value", "PLANTED-attrval-V2+/<&r8x"},
	{"collector port error argument", "PLANTED-porterr-E3+/<&t9x"},
	{"collector state value", "PLANTED-state-T5+/<&u0x"},
}

func canaryValue(slot string) string {
	for _, c := range sweepCanaries {
		if c.slot == slot {
			return c.value
		}
	}
	panic("no canary for " + slot)
}

func split(v string) string {
	half := len(v) / 2
	return `["` + jsonInner(v[:half]) + `", "` + jsonInner(v[half:]) + `"].join("")`
}

func jsonInner(s string) string {
	data, _ := json.Marshal(s)
	return string(data[1 : len(data)-1])
}

func sweepCollector() string {
	return `import { type CollectCtx, collector } from "cockpit:collect";

export default collector({
	id: "sweep",
	intervalMs: 1000,
	async run(ctx: CollectCtx) {
		const porterr = ` + split(canaryValue("collector port error argument")) + `;
		const reasons: string[] = [];
		try { await ctx.docker.exec("cache", ["valkey-cli", "CONFIG", porterr]); } catch (e) { reasons.push(String((e as Error).message)); }
		try { await ctx.http.send("GET", "http://app/" + porterr); } catch (e) { reasons.push(String((e as Error).message)); }
		try { await ctx.sql.query("main", "SELECT 1"); } catch (e) { reasons.push(String((e as Error).message)); }
		await ctx.state.set("cursor", ` + split(canaryValue("collector state value")) + `);
		ctx.signal.event({ attrs: { [` + split(canaryValue("signal attr key")) + `]: ` + split(canaryValue("signal attr value")) + `, reasons: reasons.join(" | ") } });
		await ctx.sql.query("main", "SELECT 2");
	},
});
`
}

func sweepEnv() string {
	var b strings.Builder
	b.WriteString("POSTGRES_USER=toy\nPOSTGRES_DB=toy\n")
	b.WriteString("POSTGRES_PASSWORD=" + canaryValue("sql root secret") + "\n")
	b.WriteString("APP_TOKEN=" + canaryValue("secret source and env output .env") + "\n")
	for i, slot := range []string{"runner output line", "runner step message", "signal attr key", "signal attr value", "collector port error argument", "collector state value"} {
		b.WriteString("PLANTED_" + string(rune('A'+i)) + "=" + canaryValue(slot) + "\n")
	}
	return b.String()
}

func encodings(v string) map[string]string {
	plain, _ := json.Marshal(v)
	var noHTML strings.Builder
	enc := json.NewEncoder(&noHTML)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return map[string]string{
		"raw":            v,
		"base64":         base64.StdEncoding.EncodeToString([]byte(v)),
		"base64url":      base64.URLEncoding.EncodeToString([]byte(v)),
		"base64 raw":     base64.RawStdEncoding.EncodeToString([]byte(v)),
		"base64url raw":  base64.RawURLEncoding.EncodeToString([]byte(v)),
		"hex":            hex.EncodeToString([]byte(v)),
		"hex upper":      strings.ToUpper(hex.EncodeToString([]byte(v))),
		"url query":      url.QueryEscape(v),
		"url path":       url.PathEscape(v),
		"json escaped":   string(plain[1 : len(plain)-1]),
		"json unescaped": strings.TrimSuffix(noHTML.String(), "\n")[1 : len(strings.TrimSuffix(noHTML.String(), "\n"))-1],
	}
}

type leakScan struct {
	values   []canary
	hits     []string
	masked   bool
	files    int
	vanished int
}

func (s *leakScan) text(where, text string) {
	if strings.Contains(text, "****") {
		s.masked = true
	}
	for _, c := range s.values {
		for name, form := range encodings(c.value) {
			if strings.Contains(text, form) {
				s.hits = append(s.hits, where+": "+c.slot+" ("+name+")")
			}
		}
	}
}

func (s *leakScan) tree(t *testing.T, root string, skip func(rel string) bool) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			s.vanished++
			return nil
		}
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if skip(filepath.ToSlash(rel)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		s.text("name "+rel, rel)
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			s.vanished++
			return nil
		}
		if err != nil {
			return err
		}
		s.files++
		s.text(rel, string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func sweepWorld(t *testing.T, hubLog *lockedBuffer) *world {
	base := freeBlock(t)
	w := newWorldWith(t, func(o *Options) {
		o.NoCollection, o.ScriptHost, o.Limits.Suite = false, collecttest.Command(), 10*time.Second
		o.Collect = collect.Options{DrainWait: 2 * time.Second, Heartbeat: 100 * time.Millisecond}
		o.Config.Limits.PortBase = base
		o.Log = slog.New(slog.NewTextHandler(hubLog, nil))
	}, func(f *fakedocker.Fake) {
		f.RunnerDelay, f.ContainerSecret = 3500*time.Millisecond, canaryValue("secret source and env output .env")
		f.RunnerLogs = "health › health-ok passed\ntoken " + canaryValue("runner output line") + " printed\n"
		f.EnvFiles = map[string]string{
			".env":                 sweepEnv(),
			compose.PrivateEnvName: `{"local":{"appToken":"` + jsonInner(canaryValue("private env value")) + `"}}`,
		}
		f.Steps = append(passingSteps(), contract.ResultStep{Flow: "facts", ID: "facts-note", Status: contract.StepStatusPassed, Requests: 1, Message: "saw " + canaryValue("runner step message")})
	}, func(dir string) {
		os.RemoveAll(filepath.Join(dir, "scripts", "collectors"))
		writeRepoFile(t, filepath.Join(dir, "scripts", "collectors", "sweep.ts"), sweepCollector())
	})
	w.base, w.fake.Base = base, base
	return w
}

func TestEveryPlantedCredentialIsAbsentInEveryEncodingFromTheBundleTheHubHomeAndTheHubLogAfterReportAndDestroy(t *testing.T) {
	var hubLog lockedBuffer
	w := sweepWorld(t, &hubLog)
	db := startFakePostgres(t, w.base+1)
	m := w.submit()
	if m.Verdict == "" || m.Redaction != nil {
		t.Fatalf("no clean report: state %+v notice %q redaction %+v", m.State, m.Notice, m.Redaction)
	}
	waitFor(t, "a reader password provisioned against the fake database", func() bool {
		_, readers := db.seen()
		return len(readers) > 0
	})
	roots, readers := db.seen()
	if len(roots) == 0 || roots[0] != canaryValue("sql root secret") {
		t.Fatalf("the fake database saw root passwords %d, want the planted root secret", len(roots))
	}
	values := append([]canary{}, sweepCanaries...)
	for _, r := range readers {
		values = append(values, canary{"provisioned reader password", r})
	}
	dir, err := w.hub.Store().RunDir(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	bundle := &leakScan{values: values}
	bundle.tree(t, filepath.Join(dir, report.DirReport), func(string) bool { return false })
	live := &leakScan{values: values}
	consumerTrees := map[string]bool{"src": true, "build": true, "work": true}
	runRel, _ := filepath.Rel(w.hub.Store().Home(), dir)
	live.tree(t, w.hub.Store().Home(), func(rel string) bool {
		inner, ok := strings.CutPrefix(rel, filepath.ToSlash(runRel)+"/")
		return ok && consumerTrees[strings.Split(inner, "/")[0]]
	})
	if r := w.sched.Destroy(m.ID, scheduler.Actor{By: "tester"}); r.Fail != nil {
		t.Fatal(r.Fail)
	}
	destroyed := &leakScan{values: values}
	destroyed.tree(t, w.hub.Store().Home(), func(string) bool { return false })
	if _, err := os.Stat(filepath.Join(dir, "src")); !os.IsNotExist(err) {
		t.Fatalf("destroy left the source tree: %v", err)
	}
	logs := &leakScan{values: values}
	logs.text("hub log", hubLog.String())
	logs.text("events", w.eventText())
	for _, s := range []*leakScan{bundle, live, destroyed, logs} {
		for _, hit := range s.hits {
			t.Errorf("leak: %s", hit)
		}
	}
	if !bundle.masked || !live.masked || !destroyed.masked || !logs.masked {
		t.Fatalf("a scope masked nothing: bundle %v, live home %v, destroyed home %v, logs %v", bundle.masked, live.masked, destroyed.masked, logs.masked)
	}
	if bundle.files < 5 || destroyed.files <= bundle.files || len(values) < len(sweepCanaries)+1 {
		t.Fatalf("scanned %d bundle files, %d home files after destroy, %d canaries", bundle.files, destroyed.files, len(values))
	}
	t.Logf("scanned %d canaries in %d bundle, %d live and %d destroyed home files", len(values), bundle.files, live.files, destroyed.files)
}

func TestTheLeakScanSkipsAFileThatVanishesDuringTheWalkAndStillReadsTheRest(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{"kv.json.tmp": "pending", "stay.json": "token " + canaryValue("sql root secret")} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	scan := &leakScan{values: sweepCanaries}
	scan.tree(t, root, func(rel string) bool {
		if rel == "kv.json.tmp" {
			os.Remove(filepath.Join(root, rel))
		}
		return false
	})
	if scan.vanished != 1 || scan.files != 1 || len(scan.hits) == 0 {
		t.Fatalf("vanished %d files %d hits %v", scan.vanished, scan.files, scan.hits)
	}
}
