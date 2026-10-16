function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	check("utf8 to base64", Buffer.from("hello").toString("base64") === "aGVsbG8=");
	check("base64 to utf8", Buffer.from("aGVsbG8=", "base64").toString() === "hello");
	check("multi-byte UTF-8 round trip", Buffer.from(Buffer.from("\uac00\ub098").toString("base64"), "base64").toString("utf8") === "\uac00\ub098");
	check("byte array to hex", Buffer.from([0xde, 0xad, 0xbe, 0xef]).toString("hex") === "deadbeef");
	const bytes = new Uint8Array([1, 0, 1]);
	const wrapped = Buffer.from(bytes);
	check("builds from a Uint8Array", wrapped.length === 3 && wrapped[2] === 1 && wrapped instanceof Uint8Array);
	check("reads hex", Buffer.from("00ff", "hex")[1] === 255);
}
