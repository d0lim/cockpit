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
	check("await 가 값을 푼다", (await counter.add(2)) === 4);
	check("메서드 안의 await 가 이어진다", (await counter.add(3)) === 10);
	let caught = "";
	try {
		await failing();
	} catch (error) {
		caught = (error as Error).message;
	} finally {
		caught += "+finally";
	}
	check("거부가 catch 와 finally 로 온다", caught === "boom+finally");
	const recovered = await failing().catch((error: unknown) => `recovered ${(error as Error).message}`);
	check(".catch 가 값을 돌려준다", recovered === "recovered boom");
	const order: string[] = [];
	const first = (async () => {
		order.push("a");
		await null;
		order.push("c");
	})();
	order.push("b");
	await first;
	check("await 는 마이크로태스크로 양보한다", order.join("") === "abc");
}
