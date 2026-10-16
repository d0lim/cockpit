package runnerctx

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/capture"
	cryptoport "github.com/d0lim/cockpit/internal/ports/crypto"
	httpport "github.com/d0lim/cockpit/internal/ports/http"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/ports/source"
	"github.com/d0lim/cockpit/internal/ports/totp"
)

const (
	planted = "PLANTED-pw-7f3a9c"
	rfcSeed = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	seedTwo = "SEEDTWOAAAAABBBBBCCCCCDDDDDEEEE"
)

var ctxMutations = []porttest.Mutation{
	{"ctx and its child objects are frozen", "remove the deepFreeze call in New", "red: the isFrozen assertion fails", "TestEnvAndPrivAreReadableAndFrozenAndPrivSeesTheNewSeedAfterSaveSeed"},
	{"priv shows the new value after saveSeed", "make priv a static snapshot instead of an accessor", "red: totpSecret2 is undefined after saveSeed", "TestEnvAndPrivAreReadableAndFrozenAndPrivSeesTheNewSeedAfterSaveSeed"},
	{"port errors are JS errors with a code", "throw raises a TypeError instead of h.Error", "red: code is undefined", "TestSendResolvesForAllowedURLAndRejectsDeniedOrRedirectedWithCodeAndMaskedMessage"},
	{"denial messages are masked", "set Deps.Mask to Identity", "red: PLANTED shows in the denial message", "TestSendResolvesForAllowedURLAndRejectsDeniedOrRedirectedWithCodeAndMaskedMessage"},
	{"captures without persist are secret", "capture ignores persist and writes true", "red: session is written to context.json", "TestCapturesComeBackViaGetOnlyPersistedReachContextJSONAndMissingOnesAreRejectedByName"},
	{"two-factor seeds are found in priv through the seed key table", "seed ignores SeedKeys and uses loginId as the priv key", "red: admin-1 is rejected with PORT_UNAVAILABLE", "TestTOTPIsIssuedOnlyForAccountsInSeedKeyTableAndStepIsStoredInContextJSON"},
	{"signing with a public key is rejected", "sign swallows the Keys.Sign error and returns an empty buffer", "red: public sign resolves", "TestCryptoGivesV4UUIDRandomBytesAndKeyPairsAndSignsOnlyWithPrivateKeyVerifiableInGo"},
	{"the signature covers the bytes passed in", "sign signs empty bytes instead of the data", "red: VerifyPKCS1v15 fails", "TestCryptoGivesV4UUIDRandomBytesAndKeyPairsAndSignsOnlyWithPrivateKeyVerifiableInGo"},
	{"reads outside the declared sources are rejected", "readSource swallows the Source.Read error and returns an empty string", "red: reading the other source passes", "TestSourceReadsAndListsOnlyDeclaredNamesAndRejectsOthersWithAPortCode"},
	{"sleep actually waits", "sleep resolves at once", "red: the clock difference is under 25ms", "TestClockGivesMillisAndSleepResolvesAfterTheGivenDelay"},
}

const prelude = `import { ctx, record } from "cockpit:flow";
const must = (cond: unknown, msg: string): void => { if (!cond) throw new Error(msg); };
const codeOf = (e: unknown): string => ` + "`${String((e as { code?: unknown }).code)}|${(e as Error).message}`" + `;
const catchCode = (f: () => unknown): string => { try { f(); return "none"; } catch (e) { return codeOf(e); } };
const rejectCode = async (p: Promise<unknown>): Promise<string> => { try { await p; return "none"; } catch (e) { return codeOf(e); } };
const b64url = (b: Uint8Array): string => Buffer.from(b).toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
`

type world struct {
	t        *testing.T
	e2e      string
	privPath string
	deps     *Deps
	records  []string
	changed  int
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{t: t, e2e: t.TempDir()}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/echo":
			body, _ := io.ReadAll(r.Body)
			rw.Header().Set("Content-Type", "application/json")
			rw.Header().Set("X-Echo-Auth", r.Header.Get("Authorization"))
			rw.WriteHeader(200)
			rw.Write(body)
		case "/redirect":
			rw.Header().Set("Location", "/echo")
			rw.WriteHeader(302)
		default:
			rw.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	mask := func(s string) string { return strings.ReplaceAll(s, planted, "****") }
	client, err := httpport.New(httpport.Options{
		Allowed: []string{"http://app:8080"}, Timeout: 2 * time.Second, MaxBody: 1 << 20, Mask: mask,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	w.privPath = filepath.Join(w.e2e, "e2e.private.env.json")
	if err := os.WriteFile(w.privPath, []byte(`{"local":{"adminPassword":"`+planted+`","totpSecret1":"`+rfcSeed+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	priv, err := totp.ReadPrivateEnv(w.privPath)
	if err != nil {
		t.Fatal(err)
	}
	srcRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcRoot, "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"a.json": `{"a":1}`, "b.json": `{"b":1}`} {
		if err := os.WriteFile(filepath.Join(srcRoot, "modules", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := source.Open(source.Roots{"fixtures": srcRoot}, 1<<20, mask)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reader.Close)
	cctx, err := capture.Load(w.e2e)
	if err != nil {
		t.Fatal(err)
	}
	w.deps = &Deps{
		Env:      map[string]any{"appBase": "http://app:8080", "flag": true, "nested": map[string]any{"n": 1}},
		Priv:     priv,
		PrivPath: w.privPath,
		SeedKeys: map[string]string{"admin-1": "totpSecret1", "admin-2": "totpSecret2"},
		HTTP:     client,
		Context:  cctx,
		Source:   reader,
		Keys:     cryptoport.NewKeys(mask),
		Mask:     mask,
		Changed:  func() { w.changed++ },
	}
	return w
}

func (w *world) module(h *engine.Host) (goja.Value, error) {
	ctx, err := New(h, w.deps)
	if err != nil {
		return nil, err
	}
	obj := h.Runtime().NewObject()
	if err := obj.Set("ctx", ctx); err != nil {
		return nil, err
	}
	if err := obj.Set("record", func(s string) { w.records = append(w.records, s) }); err != nil {
		return nil, err
	}
	return obj, nil
}

func (w *world) run(src string) error {
	w.t.Helper()
	root := w.t.TempDir()
	entry := filepath.Join(root, "entry.ts")
	if err := os.WriteFile(entry, []byte(prelude+src), 0o644); err != nil {
		w.t.Fatal(err)
	}
	bundle, err := engine.BuildBundle(context.Background(), engine.BundleInput{Entry: entry, Root: root, Site: engine.SiteRunner})
	if err != nil {
		w.t.Fatalf("bundle: %v", err)
	}
	eng, err := engine.New(engine.Options{
		Site:    engine.SiteRunner,
		Budget:  engine.Budget{Deadline: 30 * time.Second},
		Modules: map[string]engine.ModuleBuilder{engine.ModuleFlow: w.module},
	})
	if err != nil {
		w.t.Fatal(err)
	}
	return eng.Run(context.Background(), bundle, "default")
}

func (w *world) mustRun(src string) {
	w.t.Helper()
	if err := w.run(src); err != nil {
		w.t.Fatalf("run: %v", err)
	}
}

func (w *world) contextFile() contract.Context {
	w.t.Helper()
	raw, err := os.ReadFile(filepath.Join(w.e2e, "context.json"))
	if err != nil {
		w.t.Fatal(err)
	}
	parsed, err := contract.ReadRunnerFile[contract.Context](strings.NewReader(string(raw)))
	if err != nil {
		w.t.Fatal(err)
	}
	return *parsed
}
