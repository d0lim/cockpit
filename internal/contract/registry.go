package contract

type Place string

const (
	PlaceRunner Place = "runner"
	PlaceHub    Place = "hub"
)

type Member struct {
	Name string
	TS   string
}

type Module struct {
	Name    string
	File    string
	Place   Place
	Ctx     string
	Types   []string
	Decls   []string
	Members []Member
}

func Modules() []Module {
	return []Module{flowModule(), collectModule()}
}

const httpResponseTS = `export interface HttpHeaders {
	get(name: string): string | null;
}
export interface HttpResponse {
	readonly method: string;
	readonly url: string;
	readonly status: number;
	readonly headers: HttpHeaders;
	readonly body: string;
}`

const flowHTTPTS = `export interface SendOptions {
	headers?: Record<string, string>;
	json?: unknown;
	body?: string;
	timeoutMs?: number;
}
export interface HttpPort {
	send(method: string, url: string, opts?: SendOptions): Promise<HttpResponse>;
}
export interface CaptureOptions {
	persist?: boolean;
}`

const cryptoTS = `export interface CryptoKey {
	readonly type: "public" | "private";
}
export interface CryptoKeyPair {
	readonly publicKey: CryptoKey;
	readonly privateKey: CryptoKey;
}
export interface RsaKeyGenParams {
	name: "RSASSA-PKCS1-v1_5";
	modulusLength: number;
	publicExponent: Uint8Array;
	hash: "SHA-256" | "SHA-384" | "SHA-512";
}
export interface SubtleCrypto {
	generateKey(algorithm: RsaKeyGenParams, extractable: boolean, usages: ("sign" | "verify")[]): Promise<CryptoKeyPair>;
	sign(algorithm: "RSASSA-PKCS1-v1_5", key: CryptoKey, data: Uint8Array): Promise<ArrayBuffer>;
}
export interface CryptoPort {
	randomUUID(): string;
	getRandomValues<T extends Uint8Array | Uint16Array | Uint32Array>(array: T): T;
	readonly subtle: SubtleCrypto;
}`

const flowDefsTS = `export interface StageDef {
	id: string;
	title: string;
	notProven?: string;
}
export interface StepDef {
	id: string;
	title: string;
	kind: StepKind;
	run(ctx: Ctx): Promise<void>;
	source?: ManifestSource;
}
export interface FlowDef {
	id: string;
	title: string;
	stage: string;
	once: boolean;
	requires: string[];
	steps: StepDef[];
	source?: ManifestSource;
}
export interface SuiteDef {
	name: string;
	stages: StageDef[];
	flows: FlowDef[];
}
export function suite(def: SuiteDef): SuiteDef;
export function stage(def: StageDef): StageDef;
export function flow(def: FlowDef): FlowDef;
export function step(def: StepDef): StepDef;`

func flowModule() Module {
	return Module{
		Name:  "cockpit:flow",
		File:  "cockpit-flow.d.ts",
		Place: PlaceRunner,
		Ctx:   "Ctx",
		Types: []string{"StepKind", "ManifestSource"},
		Decls: []string{httpResponseTS, flowHTTPTS, cryptoTS, flowDefsTS},
		Members: []Member{
			{"env", "readonly env: Readonly<Record<string, unknown>>;"},
			{"priv", "readonly priv: Readonly<Record<string, string>>;"},
			{"http", "readonly http: HttpPort;"},
			{"capture", "capture(name: string, value: string, opts?: CaptureOptions): void;"},
			{"get", "get(name: string): string;"},
			{"totp", "totp(loginId: string): Promise<string>;"},
			{"saveSeed", "saveSeed(loginId: string, secret: string): void;"},
			{"crypto", "readonly crypto: CryptoPort;"},
			{"readSource", "readSource(source: string, path: string): string;"},
			{"listSource", "listSource(source: string, dir: string): string[];"},
			{"clock", "clock(): number;"},
			{"sleep", "sleep(ms: number): Promise<void>;"},
		},
	}
}

const collectPortsTS = `export type ReservedAttrKey = keyof ReservedAttrs;
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
export function collector(def: CollectorDef): CollectorDef;`

func collectModule() Module {
	return Module{
		Name:  "cockpit:collect",
		File:  "cockpit-collect.d.ts",
		Place: PlaceHub,
		Ctx:   "CollectCtx",
		Types: []string{"SignalInput", "MetricBody", "EventBody", "SpanBody", "ReservedAttrs", "Tone", "SpanStatus", "TimeSource"},
		Decls: []string{httpResponseTS, collectPortsTS},
		Members: []Member{
			{"sql", "readonly sql: SqlPort;"},
			{"docker", "readonly docker: DockerPort;"},
			{"http", "readonly http: EdgeHttpPort;"},
			{"signal", "readonly signal: SignalPort;"},
			{"state", "readonly state: StatePort;"},
		},
	}
}
