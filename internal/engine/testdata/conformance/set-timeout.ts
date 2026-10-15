function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const sleep = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

export default async function (): Promise<void> {
	const order: string[] = [];
	const late = new Promise<void>((resolve) => setTimeout(() => (order.push("late"), resolve()), 30));
	const early = new Promise<void>((resolve) => setTimeout(() => (order.push("early"), resolve()), 5));
	await Promise.all([late, early]);
	check("짧은 지연이 먼저 돈다", order.join(",") === "early,late");
	const started = Date.now();
	await sleep(20);
	check("지연만큼 기다린다", Date.now() - started >= 15);
	const passed = await new Promise<string>((resolve) => setTimeout(resolve, 0, "arg"));
	check("추가 인자를 넘긴다", passed === "arg");
}
