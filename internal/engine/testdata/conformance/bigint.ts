function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const step = BigInt(58_000_000);
	check("BigInt construction and arithmetic", step * 2n === 116_000_000n);
	check("large power", (2n ** 64n).toString() === "18446744073709551616");
	check("comparison with mixed numbers", 5n > 3 && BigInt(7) === 7n);
	check("to string and Number", String(255n) === "255" && Number(42n) === 42);
	check("asUintN truncates to 64 bits", BigInt.asUintN(64, -1n) === 18446744073709551615n);
}
