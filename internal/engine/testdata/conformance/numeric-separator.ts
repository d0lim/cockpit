function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const STEP_MS = 30_000;
const WAIT_MARGIN_MS = 300;

export default function (): void {
	check("decimal separator", STEP_MS === 30000);
	check("hex separator", 0xff_ff === 65535);
	check("separator in a fractional number", 1_000.5 === 1000.5);
	check("used in arithmetic", Math.floor(95_000 / STEP_MS) * STEP_MS + WAIT_MARGIN_MS === 90_300);
}
