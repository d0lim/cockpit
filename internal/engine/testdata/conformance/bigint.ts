function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const step = BigInt(58_000_000);
	check("BigInt 생성과 산술", step * 2n === 116_000_000n);
	check("큰 거듭제곱", (2n ** 64n).toString() === "18446744073709551616");
	check("비교와 섞인 수", 5n > 3 && BigInt(7) === 7n);
	check("문자열과 Number 로", String(255n) === "255" && Number(42n) === 42);
	check("asUintN 이 64비트로 자른다", BigInt.asUintN(64, -1n) === 18446744073709551615n);
}
