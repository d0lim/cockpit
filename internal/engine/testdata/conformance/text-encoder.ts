function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const encoder = new TextEncoder();
	const bytes = encoder.encode("\uac00a");
	check("encodes as utf-8", bytes instanceof Uint8Array && [...bytes].join(",") === "234,176,128,97");
	check("encoding is utf-8", encoder.encoding === "utf-8");
	check("empty input gives an empty array", encoder.encode().length === 0 && encoder.encode("").length === 0);
	check("astral character is 4 bytes", encoder.encode("😀").length === 4);
}
