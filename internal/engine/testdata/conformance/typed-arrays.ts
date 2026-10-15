function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const FRESH_ADDRESS_PREFIX = "0x9";

export default function (): void {
	const exponent = new Uint8Array([1, 0, 1]);
	check("Uint8Array 리터럴", exponent.length === 3 && exponent[0] === 1);
	const bytes = new Uint8Array(4);
	bytes[0] = 0xab;
	bytes[3] = 0x0c;
	const address = `${FRESH_ADDRESS_PREFIX}${[...bytes].map((b) => b.toString(16).padStart(2, "0")).join("")}`;
	check("펼쳐 hex 로 만든다", address === "0x9ab00000c");
	const word = new Uint16Array(1);
	check("Uint16Array 는 0 으로 채워진다", word[0] === 0 && (word[0] ?? 7) === 0);
	word[0] = 70_000;
	check("Uint16Array 는 16비트로 감긴다", word[0] === 4464);
	check("subarray·slice", bytes.subarray(0, 1)[0] === 0xab && bytes.slice(3)[0] === 0x0c);
	check("ArrayBuffer 공유", new Uint8Array(bytes.buffer)[3] === 0x0c);
}
