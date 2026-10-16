function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const sleep = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

export default async function (): Promise<void> {
	const order: string[] = [];
	const late = new Promise<void>((resolve) => setTimeout(() => (order.push("late"), resolve()), 30));
	const early = new Promise<void>((resolve) => setTimeout(() => (order.push("early"), resolve()), 5));
	await Promise.all([late, early]);
	check("shorter delay runs first", order.join(",") === "early,late");
	const started = Date.now();
	await sleep(20);
	check("waits for the delay", Date.now() - started >= 15);
	const passed = await new Promise<string>((resolve) => setTimeout(resolve, 0, "arg"));
	check("passes extra arguments", passed === "arg");
}
