function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const encoder = new TextEncoder();
	const bytes = encoder.encode("가a");
	check("utf-8 로 인코딩한다", bytes instanceof Uint8Array && [...bytes].join(",") === "234,176,128,97");
	check("encoding 은 utf-8 이다", encoder.encoding === "utf-8");
	check("빈 입력은 빈 배열이다", encoder.encode().length === 0 && encoder.encode("").length === 0);
	check("아스트랄 문자는 4바이트다", encoder.encode("😀").length === 4);
}
