function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const digits = 6;
	check("10 ** n", 10 ** digits === 1_000_000);
	check("음수 지수", 2 ** -1 === 0.5);
	check("오른쪽 결합", 2 ** 3 ** 2 === 512);
	let n = 3;
	n **= 2;
	check("**= 대입", n === 9);
	check("나머지와 섞인다", 1234567 % 10 ** 3 === 567);
}
