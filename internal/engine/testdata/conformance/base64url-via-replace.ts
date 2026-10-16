function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const base64url = (bytes: string | Uint8Array): string =>
	Buffer.from(bytes).toString("base64").replace(/=+$/, "").replace(/\+/g, "-").replace(/\//g, "_");

export default function (): void {
	check("strips padding", base64url("hello") === "aGVsbG8");
	check("replaces + and /", base64url(new Uint8Array([0xfb, 0xff, 0xbf])) === "-_-_");
	const header = base64url(JSON.stringify({ alg: "PS512", kid: "k1" }));
	check("encodes a JSON header URL-safe", !/[+/=]/.test(header) && Buffer.from(header, "base64").toString().includes(`"kid":"k1"`));
}
