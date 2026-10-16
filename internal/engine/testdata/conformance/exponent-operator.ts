function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const digits = 6;
	check("10 ** n", 10 ** digits === 1_000_000);
	check("negative exponent", 2 ** -1 === 0.5);
	check("right associative", 2 ** 3 ** 2 === 512);
	let n = 3;
	n **= 2;
	check("**= assignment", n === 9);
	check("mixes with remainder", 1234567 % 10 ** 3 === 567);
}
