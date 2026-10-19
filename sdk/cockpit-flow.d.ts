declare module "cockpit:flow" {
	/** Where a flow or step is defined; the runner fills it in. */
	export interface ManifestSource {
		file: string;
		line: number;
	}
	/** `request` sends requests, `wait` waits until a condition holds, `capture` stores values for later steps. */
	export type StepKind = "request" | "wait" | "capture";
	/** A response as the runner received it. `body` is the raw text, at most 8 MiB. Header names are lowercase. */
	export interface HttpResponse {
		readonly method: string;
		readonly url: string;
		readonly status: number;
		readonly headers: Readonly<Record<string, string>>;
		readonly body: string;
	}
	export interface SendOptions {
		headers?: Record<string, string>;
		/** Serialized as the request body; `content-type` defaults to `application/json`. */
		json?: unknown;
		/** A raw request body. Ignored when `json` is set. */
		body?: string;
		/** Timeout for this request. Defaults to 30 seconds. */
		timeoutMs?: number;
	}
	export interface HttpPort {
		/** Sends one request and resolves with the response, whatever its status. The URL must be absolute and its origin (scheme://host[:port]) must appear as a value of its own in env.json; an IP address or a URL with a path is not an origin. Other origins and redirect responses are refused. */
		send(method: string, url: string, opts?: SendOptions): Promise<HttpResponse>;
	}
	export interface CaptureOptions {
		/** Also keep the value in context.json, so later runs and `--flow` runs in the same environment can read it. */
		persist?: boolean;
	}
	export interface CryptoKey {
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
	/** Randomness and RSA signing. Keys are opaque handles and never leave the runner. */
	export interface CryptoPort {
		randomUUID(): string;
		getRandomValues<T extends Uint8Array | Uint16Array | Uint32Array>(array: T): T;
		readonly subtle: SubtleCrypto;
	}
	/** A group of flows in the results. `notProven` marks a stage this environment cannot prove, with the reason. */
	export interface StageDef {
		id: string;
		title: string;
		notProven?: string;
	}
	export interface StepDef {
		id: string;
		title: string;
		/** What the step does; used only for reading the results. */
		kind: StepKind;
		/** Runs the step. The step fails when this throws; the message is masked and cut to 500 characters. */
		run(ctx: Ctx): Promise<void>;
		source?: ManifestSource;
	}
	export interface FlowDef {
		/** Lowercase letters, digits and hyphens, starting with a letter; 2 to 41 characters. */
		id: string;
		title: string;
		/** The id of a stage declared in the suite. */
		stage: string;
		/** Runs at most once per environment, for setup such as creating the first admin. With `--flow`, a once target that already ran is refused, and a once require that passed is reused instead of rerun. */
		once: boolean;
		/** Flows that must pass first. When one fails or is skipped, this flow is skipped. */
		requires: string[];
		steps: StepDef[];
		source?: ManifestSource;
	}
	export interface SuiteDef {
		/** Must equal the file name: the suite in suites/smoke.ts is named `smoke`. */
		name: string;
		stages: StageDef[];
		flows: FlowDef[];
	}
	/** Declares the suite. Default-export the result from suites/<name>.ts. */
	export function suite(def: SuiteDef): SuiteDef;
	/** Declares a stage. */
	export function stage(def: StageDef): StageDef;
	/** Declares a flow: steps run in order, and the first step that throws stops the flow. */
	export function flow(def: FlowDef): FlowDef;
	/** Declares a step. */
	export function step(def: StepDef): StepDef;
	export interface PollOptions {
		/** Names the condition in the failure message. */
		what: string;
		deadlineMs: number;
		intervalMs: number;
		/** Pass `ctx.clock`. */
		now: () => number;
		/** Pass `ctx.sleep`. */
		sleep: (ms: number) => Promise<void>;
	}
	/** Formats `METHOD /path` without the query, for failure messages. */
	export function target(method: string, url: string): string;
	/** Throws unless the status is one of `expected`. The message names the method and path, never the query or the body. */
	export function expectStatus(res: HttpResponse, ...expected: number[]): void;
	/** Parses the body as JSON. Throws without echoing the body when it is not JSON. */
	export function json<T = any>(res: HttpResponse): T;
	/** Calls `attempt` until `done` holds and resolves with the last value and the attempt count. The first attempt runs at once; it throws when the next attempt would start after the deadline. */
	export function poll<T>(attempt: () => Promise<T>, done: (value: T) => boolean, opts: PollOptions): Promise<{ value: T; attempts: number }>;
	/** The ports a step can use. Everything a flow touches goes through here; there is no fetch, file system or Node API. */
	export interface Ctx {
		/** The public env, read from env.json. */
		readonly env: Readonly<Record<string, unknown>>;
		/** The `local` section of the private env. Every value is masked as **** in the console, failure messages and output files. */
		readonly priv: Readonly<Record<string, string>>;
		/** Requests to the origins listed in env.json. */
		readonly http: HttpPort;
		/** Stores a value for later steps and flows in this run. Captured values are masked in all output. */
		capture(name: string, value: string, opts?: CaptureOptions): void;
		/** Reads a captured value. Throws, failing the step, when the value was never captured. */
		get(name: string): string;
		/** Returns a TOTP code for an account in env.json `totpSeedKeys`, whose seed is in the private env. It does not return the same code twice for one account, and waits for the next time step when it has to. */
		totp(loginId: string): Promise<string>;
		/** Stores a TOTP seed for an account in env.json `totpSeedKeys` into the private env file. */
		saveSeed(loginId: string, secret: string): void;
		/** Randomness and RSA signing. */
		readonly crypto: CryptoPort;
		/** Reads a regular file under /work/source/<source>, at most 4 MiB. Links and paths outside the source are refused. */
		readSource(source: string, path: string): string;
		/** Lists the entries of a directory under /work/source/<source>; an absent directory lists as empty. */
		listSource(source: string, dir: string): string[];
		/** The current time in milliseconds since the epoch. */
		clock(): number;
		/** Pauses for `ms` milliseconds. */
		sleep(ms: number): Promise<void>;
	}
}
