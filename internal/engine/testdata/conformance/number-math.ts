function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const EPOCH_DIGITS = 10;

export default function (): void {
	check("Number.isInteger", Number.isInteger(3) && !Number.isInteger(3.5) && !Number.isInteger("3"));
	check("Number.isNaN", Number.isNaN(Number("x")) && !Number.isNaN(1));
	check("Number conversion", Number("12") === 12 && Number("") === 0 && Number("0x10") === 16);
	check("Math.floor, trunc, max, min", Math.floor(-1.5) === -2 && Math.trunc(-1.5) === -1 && Math.max(1, 3) === 3 && Math.min(1, 3) === 1);
	const stamp = String(Math.trunc(1_760_000_000_123.9)).padStart(EPOCH_DIGITS, "0").slice(-EPOCH_DIGITS);
	check("truncating a timestamp", stamp === "0000000123");
	check("toString(16) and padStart", [1, 255].map((b) => b.toString(16).padStart(2, "0")).join("") === "01ff");
	check("remainder operator", 7 % 3 === 1 && -7 % 3 === -1);
	check("Math.random range", Math.random() >= 0 && Math.random() < 1);
}
