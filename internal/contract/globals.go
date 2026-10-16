package contract

const GlobalsFile = "cockpit-globals.d.ts"

const GlobalsTS = `type BufferEncoding = "utf8" | "utf-8" | "hex" | "base64";

interface Buffer extends Uint8Array {
	toString(encoding?: BufferEncoding, start?: number, end?: number): string;
	equals(other: Uint8Array): boolean;
}

declare var Buffer: {
	from(data: string, encoding?: BufferEncoding): Buffer;
	from(data: ArrayBuffer | ArrayBufferView | ArrayLike<number>): Buffer;
	alloc(size: number, fill?: string | number): Buffer;
	concat(list: readonly Uint8Array[], totalLength?: number): Buffer;
};

declare class TextEncoder {
	readonly encoding: "utf-8";
	encode(input?: string): Uint8Array;
}

declare class URLSearchParams {
	constructor(init?: string | Record<string, string> | Iterable<[string, string]> | URLSearchParams);
	append(name: string, value: string): void;
	delete(name: string): void;
	get(name: string): string | null;
	getAll(name: string): string[];
	has(name: string): boolean;
	set(name: string, value: string): void;
	sort(): void;
	forEach(callback: (value: string, name: string) => void): void;
	keys(): IterableIterator<string>;
	values(): IterableIterator<string>;
	entries(): IterableIterator<[string, string]>;
	[Symbol.iterator](): IterableIterator<[string, string]>;
	toString(): string;
}

declare class URL {
	constructor(url: string, base?: string | URL);
	hash: string;
	host: string;
	hostname: string;
	href: string;
	readonly origin: string;
	password: string;
	pathname: string;
	port: string;
	protocol: string;
	search: string;
	readonly searchParams: URLSearchParams;
	username: string;
	toString(): string;
	toJSON(): string;
}

declare var console: {
	log(...data: unknown[]): void;
	info(...data: unknown[]): void;
	debug(...data: unknown[]): void;
	warn(...data: unknown[]): void;
	error(...data: unknown[]): void;
};

declare function setTimeout(callback: (...args: unknown[]) => void, ms?: number, ...args: unknown[]): number;
declare function atob(data: string): string;
declare function btoa(data: string): string;
`
