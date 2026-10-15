function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

class Store {
	private readonly names: string[] = [];

	constructor(seed: string[]) {
		seed.forEach((name) => this.names.push(name.toUpperCase()));
	}

	list(): string[] {
		return this.names.map((name) => `${name}:${this.names.length}`);
	}
}

const counter = (): (() => number) => {
	let count = 0;
	return () => ++count;
};

export default function (): void {
	check("화살표 콜백의 this 는 바깥 this 다", new Store(["a", "b"]).list().join(",") === "A:2,B:2");
	const next = counter();
	next();
	check("클로저가 상태를 붙든다", next() === 2 && counter()() === 1);
	const pair = (id: string) => ({ id, ok: true });
	check("객체 리터럴을 돌려주는 화살표", pair("x").id === "x" && pair("x").ok);
	const result = ((n: number) => n * 2)(21);
	check("즉시 부르는 화살표", result === 42);
	const pick = (name: string) => (row: { name: string }) => row.name === name;
	check("화살표를 돌려주는 화살표", [{ name: "a" }, { name: "b" }].filter(pick("b")).length === 1);
}
