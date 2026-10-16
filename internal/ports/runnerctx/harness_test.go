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
	{"ctx 와 하위 객체 동결", "New 의 deepFreeze 호출을 지운다", "빨강: isFrozen 단정이 깨진다", "Test환경과_자격은_읽히고_얼어_있으며_saveSeed_뒤_priv_가_새_값을_본다"},
	{"priv 는 saveSeed 뒤 새 값", "priv 를 접근자 대신 정적 스냅샷으로 둔다", "빨강: saveSeed 뒤 totpSecret2 가 undefined 다", "Test환경과_자격은_읽히고_얼어_있으며_saveSeed_뒤_priv_가_새_값을_본다"},
	{"포트 오류는 code 가 붙은 JS 오류", "throw 가 h.Error 대신 TypeError 를 던진다", "빨강: code 가 undefined 다", "Test허용_주소의_send_는_응답을_돌려주고_거부와_리다이렉트는_code_가_붙은_오류로_거절되며_문장은_가려진다"},
	{"거부 문장 마스킹", "Deps.Mask 를 Identity 로 바꾼다", "빨강: PLANTED 가 거부 문장에 보인다", "Test허용_주소의_send_는_응답을_돌려주고_거부와_리다이렉트는_code_가_붙은_오류로_거절되며_문장은_가려진다"},
	{"persist 없는 캡처는 비밀", "capture 가 persist 를 무시하고 true 로 적는다", "빨강: session 이 context.json 에 쓰인다", "Test캡처는_get_으로_돌아오고_persist_만_context_json_에_남으며_없는_캡처는_이름을_말하며_거부된다"},
	{"2팩터 시드는 priv 에서 키 표로 찾는다", "seed 가 SeedKeys 를 무시하고 loginId 를 priv 키로 쓴다", "빨강: admin-1 이 PORT_UNAVAILABLE 로 거부된다", "Test2팩터_코드는_시드_키_표의_계정만_발급되고_스텝이_context_json_에_남는다"},
	{"공개키로 서명 거부", "sign 이 Keys.Sign 의 오류를 삼키고 빈 버퍼를 돌려준다", "빨강: public sign 이 resolve 된다", "Test암호_포트는_v4_UUID와_난수와_키쌍을_내고_개인키로만_서명하며_Go_공개키로_검증된다"},
	{"서명은 넘긴 바이트에 대한 것", "sign 이 데이터 대신 빈 바이트를 서명한다", "빨강: VerifyPKCS1v15 가 실패한다", "Test암호_포트는_v4_UUID와_난수와_키쌍을_내고_개인키로만_서명하며_Go_공개키로_검증된다"},
	{"소스 밖 거부", "readSource 가 Source.Read 의 오류를 삼키고 빈 문자열을 돌려준다", "빨강: other 소스 읽기가 통과한다", "Test소스는_선언된_이름의_파일만_읽고_나열하며_그_밖은_code_가_붙은_오류로_거부한다"},
	{"sleep 은 실제로 기다린다", "sleep 이 바로 resolve 한다", "빨강: clock 차가 25ms 미만이다", "Test시계는_밀리초를_주고_sleep_은_그만큼_뒤에_푼다"},
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
		w.t.Fatalf("번들: %v", err)
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
		w.t.Fatalf("런: %v", err)
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
