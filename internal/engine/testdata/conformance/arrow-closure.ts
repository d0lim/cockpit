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
	check("this in an arrow callback is the outer this", new Store(["a", "b"]).list().join(",") === "A:2,B:2");
	const next = counter();
	next();
	check("closure keeps its state", next() === 2 && counter()() === 1);
	const pair = (id: string) => ({ id, ok: true });
	check("arrow returning an object literal", pair("x").id === "x" && pair("x").ok);
	const result = ((n: number) => n * 2)(21);
	check("immediately invoked arrow", result === 42);
	const pick = (name: string) => (row: { name: string }) => row.name === name;
	check("arrow returning an arrow", [{ name: "a" }, { name: "b" }].filter(pick("b")).length === 1);
}
