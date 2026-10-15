interface Probe {
	record(text: string): void;
	planted: { map: string; inlineMap: string };
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

function stackOf(code: string): string {
	try {
		eval(code);
		return "no throw";
	} catch (error) {
		return String((error as Error).stack);
	}
}

export default function (): void {
	const p = probe();
	p.record(stackOf(`1;\nthrow new Error("disk");\n//# sourceMappingURL=${p.planted.map}`));
	p.record(stackOf(`1;\nthrow new Error("inline");\n//# sourceMappingURL=${p.planted.inlineMap}`));
}
