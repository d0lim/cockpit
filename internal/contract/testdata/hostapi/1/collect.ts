import {
	type Attrs,
	type AttrValue,
	type CollectCtx,
	collector,
	type CollectorDef,
	type DockerPort,
	type EdgeHttpPort,
	type EventBody,
	type ExecOutput,
	type HttpResponse,
	type LogOptions,
	type MetricBody,
	type ReservedAttrKey,
	type ReservedAttrs,
	type ServiceState,
	type SignalInput,
	type SignalPort,
	type SpanBody,
	type SpanLink,
	type SpanStatus,
	type SqlOptions,
	type SqlPort,
	type SqlResult,
	type StatePort,
	type TextOutput,
	type TimeSource,
	type Tone,
} from "cockpit:collect";

const values: AttrValue[] = ["text", 1, true];
const keys: ReservedAttrKey[] = ["table", "status", "label", "tone"];
const tones: Tone[] = ["ok", "warn", "bad", "idle", "info"];
const sources: TimeSource[] = ["host", "container", "runner"];
const statuses: SpanStatus[] = ["ok", "error", "incomplete"];
const reserved: ReservedAttrs = { table: "orders", status: "open", label: "Orders", tone: "ok" };
const attrs: Attrs = { ...reserved, extra: values[0] ?? "text", count: 2, flag: false };
const input: SignalInput = { ts: "2026-10-15T00:00:00.000Z", tsSource: "container", id: "s-1", rev: 1, attrs };

async function useSql(sql: SqlPort): Promise<number> {
	const opts: SqlOptions = { maxRows: 10, maxBytes: 1024, timeoutMs: 1000, params: ["a", 1, true, null] };
	const result: SqlResult = await sql.query("main", "select 1", opts);
	const bare: SqlResult = await sql.query("main", "select 1");
	const columns: string[] = result.columns;
	const rows: unknown[][] = result.rows;
	const truncated: boolean = result.truncated;
	return columns.length + rows.length + bare.rows.length + (truncated ? 1 : 0);
}

async function useDocker(docker: DockerPort): Promise<number> {
	const services: ServiceState[] = await docker.ps();
	const first: ServiceState | undefined = services[0];
	const service: string = first?.service ?? "";
	const state: string = first?.state ?? "";
	const health: string | undefined = first?.health;
	const exitCode: number | undefined = first?.exitCode;
	const logOptions: LogOptions = { since: "1m", tail: 10, maxBytes: 1024 };
	const logs: TextOutput = await docker.logs("app", logOptions);
	const allLogs: TextOutput = await docker.logs("app");
	const text: string = logs.text;
	const cut: boolean = logs.truncated;
	const stats: Record<string, string> = await docker.stats("app");
	const file: TextOutput = await docker.file("app", "/etc/hostname", { maxBytes: 64 });
	const bareFile: TextOutput = await docker.file("app", "/etc/hostname");
	const out: ExecOutput = await docker.exec("cache", ["valkey-cli", "PING"], { timeoutMs: 1000, maxBytes: 64 });
	const bareOut: ExecOutput = await docker.exec("cache", ["valkey-cli", "PING"]);
	const code: number = out.exitCode;
	const stdout: string = out.stdout;
	const stderr: string = out.stderr;
	const execCut: boolean = out.truncated;
	return [service, state, health ?? "", text, stdout, stderr, allLogs.text, file.text, bareFile.text, bareOut.stdout].join("").length + code + (exitCode ?? 0) + Object.keys(stats).length + (cut || execCut ? 1 : 0);
}

async function useHttp(http: EdgeHttpPort): Promise<number> {
	const res: HttpResponse = await http.send("GET", "http://app/health", { headers: { accept: "text/plain" }, timeoutMs: 1000 });
	const head: HttpResponse = await http.send("HEAD", "http://app/health");
	const method: string = res.method;
	const url: string = res.url;
	const status: number = res.status;
	const headers: Readonly<Record<string, string>> = res.headers;
	const body: string = res.body;
	return status + head.status + method.length + url.length + Object.keys(headers).length + body.length;
}

function useSignal(signal: SignalPort, n: number): void {
	const metric: MetricBody = { name: "rows", value: n };
	const event: EventBody = { body: "seen" };
	const link: SpanLink = { traceId: "t-0", spanId: "s-0" };
	const span: SpanBody = { traceId: "t-1", parentId: "s-0", name: "order", start: "2026-10-15T00:00:00.000Z", end: "2026-10-15T00:00:01.000Z", status: statuses[0] ?? "ok", links: [link] };
	const emitted: void = signal.metric({ ...input, ...metric });
	signal.metric(metric);
	signal.event({ ...input, ...event });
	signal.event({});
	signal.span({ ...input, ...span });
	signal.span({ traceId: "t-2", name: "bare", start: "2026-10-15T00:00:00.000Z", status: "incomplete" });
	console.log(emitted, keys, tones, sources);
}

async function useState(state: StatePort): Promise<string> {
	const stored: Promise<void> = state.set("cursor", "1");
	await stored;
	const read: string | undefined = await state.get("cursor");
	return read ?? "";
}

async function run(ctx: CollectCtx): Promise<void> {
	const total = (await useSql(ctx.sql)) + (await useDocker(ctx.docker)) + (await useHttp(ctx.http));
	useSignal(ctx.signal, total);
	await useState(ctx.state);
}

const timed: CollectorDef = collector({ id: "orders", intervalMs: 5000, run });
const once: CollectorDef = { id: "once", run };

export default [timed, once];
