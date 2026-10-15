function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	check("utf8 → base64", Buffer.from("hello").toString("base64") === "aGVsbG8=");
	check("base64 → utf8", Buffer.from("aGVsbG8=", "base64").toString() === "hello");
	check("한글 왕복", Buffer.from(Buffer.from("가나").toString("base64"), "base64").toString("utf8") === "가나");
	check("바이트 배열 → hex", Buffer.from([0xde, 0xad, 0xbe, 0xef]).toString("hex") === "deadbeef");
	const bytes = new Uint8Array([1, 0, 1]);
	const wrapped = Buffer.from(bytes);
	check("Uint8Array 로 만든다", wrapped.length === 3 && wrapped[2] === 1 && wrapped instanceof Uint8Array);
	check("hex 를 읽는다", Buffer.from("00ff", "hex")[1] === 255);
}
