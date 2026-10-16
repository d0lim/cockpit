function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const doubled = async (value: number): Promise<number> => value * 2;

class Counter {
	total = 0;

	async add(value: number): Promise<number> {
		this.total += await doubled(value);
		return this.total;
	}
}

async function failing(): Promise<never> {
	throw new Error("boom");
}

export default async function (): Promise<void> {
	const counter = new Counter();
	check("await unwraps the value", (await counter.add(2)) === 4);
	check("await inside a method resumes", (await counter.add(3)) === 10);
	let caught = "";
	try {
		await failing();
	} catch (error) {
		caught = (error as Error).message;
	} finally {
		caught += "+finally";
	}
	check("rejection reaches catch and finally", caught === "boom+finally");
	const recovered = await failing().catch((error: unknown) => `recovered ${(error as Error).message}`);
	check(".catch returns a value", recovered === "recovered boom");
	const order: string[] = [];
	const first = (async () => {
		order.push("a");
		await null;
		order.push("c");
	})();
	order.push("b");
	await first;
	check("await yields to a microtask", order.join("") === "abc");
}
