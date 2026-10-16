declare module "cockpit:flow" {
	export interface ManifestSource {
		file: string;
		line: number;
	}
	export type StepKind = "request" | "wait" | "capture";
	export interface HttpResponse {
		readonly method: string;
		readonly url: string;
		readonly status: number;
		readonly headers: Readonly<Record<string, string>>;
		readonly body: string;
	}
	export interface SendOptions {
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
	export interface CryptoPort {
		randomUUID(): string;
		getRandomValues<T extends Uint8Array | Uint16Array | Uint32Array>(array: T): T;
		readonly subtle: SubtleCrypto;
	}
	export interface StageDef {
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
	export function step(def: StepDef): StepDef;
	export interface PollOptions {
		what: string;
		deadlineMs: number;
		intervalMs: number;
		now: () => number;
		sleep: (ms: number) => Promise<void>;
	}
	export function target(method: string, url: string): string;
	export function expectStatus(res: HttpResponse, ...expected: number[]): void;
	export function json<T = any>(res: HttpResponse): T;
	export function poll<T>(attempt: () => Promise<T>, done: (value: T) => boolean, opts: PollOptions): Promise<{ value: T; attempts: number }>;
	export interface Ctx {
		readonly env: Readonly<Record<string, unknown>>;
		readonly priv: Readonly<Record<string, string>>;
		readonly http: HttpPort;
		capture(name: string, value: string, opts?: CaptureOptions): void;
		get(name: string): string;
		totp(loginId: string): Promise<string>;
		saveSeed(loginId: string, secret: string): void;
		readonly crypto: CryptoPort;
		readSource(source: string, path: string): string;
		listSource(source: string, dir: string): string[];
		clock(): number;
		sleep(ms: number): Promise<void>;
	}
}
