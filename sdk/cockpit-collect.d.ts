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
	/** A response as the runner received it. `body` is the raw text, at most 8 MiB. Header names are lowercase. */
	export interface HttpResponse {
		readonly method: string;
		readonly url: string;
		readonly status: number;
		readonly headers: Readonly<Record<string, string>>;
		readonly body: string;
	}
	export type ReservedAttrKey = keyof ReservedAttrs;
	export interface SqlOptions {
		maxRows?: number;
		maxBytes?: number;
		timeoutMs?: number;
		/** Values bound to the placeholders of the statement in order: `?` for MySQL, `$1` to `$n` for PostgreSQL. They travel to the driver as parameters, never as SQL text, so ids from earlier steps need no quoting. Only strings, numbers, booleans and null are accepted; `undefined`, holes, objects and arrays are PORT_BAD_ARGS, and so is a count that does not match the placeholders. At most 100 values and 64 KiB, counting the bytes of each string and 8 for any other value (PORT_LIMIT). Pass integers above 2^53 as strings. */
		params?: (string | number | boolean | null)[];
	}
	export interface SqlResult {
		columns: string[];
		rows: unknown[][];
		truncated: boolean;
	}
	export interface SqlPort {
		/** Runs one read statement (select, with, show, explain, describe, table, values) against a database of the declaration, in a read-only transaction that is rolled back. A second statement, a write, or a MySQL `INTO OUTFILE` is PORT_DENIED; the refusal never quotes the statement. */
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
		/** Sends one GET or HEAD request to an edge port of this run and resolves with the response. The URL is http://<edge name>/path, where the edge name is an offset name from the project declaration; ports, IP addresses, other hosts and redirect responses are refused. */
		send(method: "GET" | "HEAD", url: string, opts?: { headers?: Record<string, string>; timeoutMs?: number }): Promise<HttpResponse>;
	}
	/** Emits signals into this run's stream under the collector's id. Every string is masked before it is stored. The `runner` time source is reserved for the hub. */
	export interface SignalPort {
		metric(input: SignalInput & MetricBody): void;
		event(input: SignalInput & EventBody): void;
		span(input: SignalInput & SpanBody): void;
	}
	export interface StatePort {
		/** Reads a value this collector stored earlier in this run, or `undefined`. */
		get(key: string): Promise<string | undefined>;
		/** Stores a string of at most 64 KiB under a key of at most 256 characters; the state survives hub restarts. */
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
