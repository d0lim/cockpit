import {
	type CaptureOptions,
	type CryptoKey,
	type CryptoKeyPair,
	type CryptoPort,
	type Ctx,
	expectStatus,
	flow,
	type FlowDef,
	type HttpPort,
	type HttpResponse,
	json,
	type ManifestSource,
	poll,
	type PollOptions,
	type RsaKeyGenParams,
	type SendOptions,
	stage,
	type StageDef,
	step,
	type StepDef,
	type StepKind,
	type SubtleCrypto,
	suite,
	type SuiteDef,
	target,
} from "cockpit:flow";

const kinds: StepKind[] = ["request", "wait", "capture"];
const where: ManifestSource = { file: "suites/compat.ts", line: 1 };

async function useResponse(res: HttpResponse): Promise<string> {
	const method: string = res.method;
	const url: string = res.url;
	const status: number = res.status;
	const headers: Readonly<Record<string, string>> = res.headers;
	const body: string = res.body;
	expectStatus(res, 200, 201);
	const parsed: { ok: boolean } = json<{ ok: boolean }>(res);
	const untyped: unknown = json(res);
	const label: string = target(method, url);
	return `${label} ${status} ${headers["content-type"] ?? ""} ${body.length} ${parsed.ok} ${String(untyped)}`;
}

async function useHttp(http: HttpPort): Promise<HttpResponse> {
	const opts: SendOptions = { headers: { accept: "application/json" }, json: { a: 1 }, body: "raw", timeoutMs: 1000 };
	const bare: Promise<HttpResponse> = http.send("GET", "http://app/health");
	await bare;
	return http.send("POST", "http://app/items", opts);
}

async function useCrypto(crypto: CryptoPort): Promise<string> {
	const id: string = crypto.randomUUID();
	const bytes: Uint8Array = crypto.getRandomValues(new Uint8Array(8));
	const words: Uint16Array = crypto.getRandomValues(new Uint16Array(2));
	const longs: Uint32Array = crypto.getRandomValues(new Uint32Array(2));
	const subtle: SubtleCrypto = crypto.subtle;
	const params: RsaKeyGenParams = { name: "RSASSA-PKCS1-v1_5", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" };
	const hashes: RsaKeyGenParams["hash"][] = ["SHA-256", "SHA-384", "SHA-512"];
	const pair: CryptoKeyPair = await subtle.generateKey(params, false, ["sign", "verify"]);
	const publicKey: CryptoKey = pair.publicKey;
	const privateKey: CryptoKey = pair.privateKey;
	const type: "public" | "private" = privateKey.type;
	const signature: ArrayBuffer = await subtle.sign("RSASSA-PKCS1-v1_5", privateKey, bytes);
	return `${id} ${words.length} ${longs.length} ${hashes.length} ${publicKey.type} ${type} ${signature.byteLength}`;
}

async function useCtx(ctx: Ctx): Promise<void> {
	const env: Readonly<Record<string, unknown>> = ctx.env;
	const priv: Readonly<Record<string, string>> = ctx.priv;
	const res = await useHttp(ctx.http);
	const summary = await useResponse(res);
	const persist: CaptureOptions = { persist: true };
	const captured: void = ctx.capture("summary", summary, persist);
	ctx.capture("plain", summary);
	const marked: void = ctx.ref("order", "ord-1");
	const read: string = ctx.get("summary");
	const code: string = await ctx.totp("user-1");
	const saved: void = ctx.saveSeed("user-1", "seed");
	const signed: string = await useCrypto(ctx.crypto);
	const source: string = ctx.readSource("fixtures", "seed.json");
	const entries: string[] = ctx.listSource("fixtures", ".");
	const now: number = ctx.clock();
	const slept: Promise<void> = ctx.sleep(1);
	await slept;
	const pollOptions: PollOptions = { what: "ready", deadlineMs: 1000, intervalMs: 10, now: ctx.clock, sleep: ctx.sleep };
	const polled: { value: number; attempts: number } = await poll(async () => ctx.clock(), (value: number) => value > 0, pollOptions);
	console.log(env, priv, captured, marked, read, code, saved, signed, source, entries, now, polled.value, polled.attempts);
}

const ping: StepDef = step({ id: "ping", title: "Ping", kind: kinds[0] ?? "request", run: useCtx, source: where });
const declared: StepDef = { id: "plain", title: "Plain", kind: "wait", async run(ctx: Ctx) { await ctx.sleep(1); } };
const basic: StageDef = stage({ id: "basic", title: "Basic", notProven: "no host" });
const plainStage: StageDef = { id: "plain", title: "Plain" };
const compat: FlowDef = flow({ id: "compat", title: "Compat", stage: "basic", once: true, requires: ["setup"], steps: [ping, declared], source: where });
const setup: FlowDef = { id: "setup", title: "Setup", stage: "plain", once: false, requires: [], steps: [] };
const def: SuiteDef = suite({ name: "compat", stages: [basic, plainStage], flows: [setup, compat] });

export default def;
