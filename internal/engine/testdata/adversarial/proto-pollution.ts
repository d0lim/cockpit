interface Probe {
	record(text: string): void;
	maxRows(options: object): void;
}

const SITES = ["cockpit:flow", "cockpit:collect"];

function probe(): Probe {
	for (const name of SITES) {
		try {
			return require(name) as Probe;
		} catch {}
	}
	throw new Error("no probe module");
}

export default function (): void {
	const p = probe();
	p.record(`pollute ${Reflect.set(Object.prototype, "maxRows", 1e9)}`);
	p.record(`inherited ${String(({} as Record<string, unknown>).maxRows)}`);
	p.maxRows({});
	p.maxRows(Object.create({ maxRows: 1e9 }));
	p.maxRows({ maxRows: 5 });
	p.record(`overwrite ${Reflect.set(p, "maxRows", () => undefined)}`);
	p.maxRows({ maxRows: 7 });
	p.record(`frozen ${[p, Object.prototype, Array.prototype, Function.prototype, Promise.prototype].map(Object.isFrozen).join(" ")}`);
}
