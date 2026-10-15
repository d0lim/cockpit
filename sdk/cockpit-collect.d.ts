declare module "cockpit:collect" {
	export type AttrValue = string | number | boolean;
	export type Attrs = ReservedAttrs & { [key: string]: AttrValue };
	export interface EventBody {
		body?: string;
	}
	export interface MetricBody {
		name: string;
		value: number;
	}
	export interface ReservedAttrs {
		table?: string;
		status?: string;
		label?: string;
		tone?: Tone;
	}
	export interface SignalInput {
		ts?: string;
		tsSource?: TimeSource;
		id?: string;
		rev?: number;
		attrs?: Attrs;
	}
	export interface SpanBody {
		traceId: string;
		parentId?: string;
		name: string;
		start: string;
		end?: string;
		status: SpanStatus;
		links?: SpanLink[];
	}
	export interface SpanLink {
		traceId: string;
		spanId: string;
	}
	export type SpanStatus = "ok" | "error" | "incomplete";
	export type TimeSource = "host" | "container" | "runner";
	export type Tone = "ok" | "warn" | "bad" | "idle" | "info";
	export interface HttpHeaders {
		get(name: string): string | null;
	}
	export interface HttpResponse {
		readonly method: string;
		readonly url: string;
		readonly status: number;
		readonly headers: HttpHeaders;
		readonly body: string;
	}
	export type ReservedAttrKey = keyof ReservedAttrs;
	export interface SqlOptions {
		maxRows?: number;
		maxBytes?: number;
		timeoutMs?: number;
	}
	export interface SqlResult {
		columns: string[];
		rows: unknown[][];
		truncated: boolean;
	}
	export interface SqlPort {
		query(db: string, statement: string, opts?: SqlOptions): Promise<SqlResult>;
	}
	export interface ServiceState {
		service: string;
		state: string;
		health?: string;
	}
	export interface TextOutput {
		text: string;
		truncated: boolean;
	}
	export interface LogOptions {
		since?: string;
		tail?: number;
		maxBytes?: number;
	}
	export interface ExecOutput {
		exitCode: number;
		stdout: string;
		stderr: string;
		truncated: boolean;
	}
	export interface DockerPort {
		ps(): Promise<ServiceState[]>;
		logs(service: string, opts?: LogOptions): Promise<TextOutput>;
		stats(service: string): Promise<Record<string, string>>;
		file(service: string, path: string, opts?: { maxBytes?: number }): Promise<TextOutput>;
		exec(service: string, argv: string[], opts?: { timeoutMs?: number; maxBytes?: number }): Promise<ExecOutput>;
	}
	export interface EdgeHttpPort {
		send(method: "GET" | "HEAD", url: string, opts?: { headers?: Record<string, string>; timeoutMs?: number }): Promise<HttpResponse>;
	}
	export interface SignalPort {
		metric(input: SignalInput & MetricBody): void;
		event(input: SignalInput & EventBody): void;
		span(input: SignalInput & SpanBody): void;
	}
	export interface StatePort {
		get(key: string): Promise<string | undefined>;
		set(key: string, value: string): Promise<void>;
	}
	export interface CollectorDef {
		id: string;
		intervalMs?: number;
		run(ctx: CollectCtx): Promise<void>;
	}
	export function collector(def: CollectorDef): CollectorDef;
	export interface CollectCtx {
		readonly sql: SqlPort;
		readonly docker: DockerPort;
		readonly http: EdgeHttpPort;
		readonly signal: SignalPort;
		readonly state: StatePort;
	}
}
