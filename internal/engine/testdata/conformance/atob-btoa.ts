function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

function throws(fn: () => unknown): boolean {
	try {
		fn();
		return false;
	} catch {
		return true;
	}
}

export default function (): void {
	check("btoa", btoa("hello") === "aGVsbG8=");
	check("atob", atob("aGVsbG8=") === "hello");
	check("accepts input without padding and with spaces", atob("aGVs bG8") === "hello");
	check("Latin-1 round trip", atob(btoa("ÿ\u0000")) === "ÿ\u0000" && btoa("ÿ") === "/w==");
	check("characters outside Latin-1 throw", throws(() => btoa("\uac00")));
	check("non-base64 input throws", throws(() => atob("a")) && throws(() => atob("a*bc")));
}
