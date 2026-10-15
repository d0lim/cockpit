function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const STEP_MS = 30_000;
const WAIT_MARGIN_MS = 300;

export default function (): void {
	check("10진 구분자", STEP_MS === 30000);
	check("16진 구분자", 0xff_ff === 65535);
	check("소수 구분자", 1_000.5 === 1000.5);
	check("계산에 쓰인다", Math.floor(95_000 / STEP_MS) * STEP_MS + WAIT_MARGIN_MS === 90_300);
}
