interface Probe {
	record(text: string): void;
	foreign: string;
	planted: { absolute: string; relativeJson: string };
}

const SITES = ["cockpit:flow", "cockpit:collect"];
const load = (name: string): unknown => require(name);

function probe(): Probe {
	for (const name of SITES) {
		try {
			return load(name) as Probe;
		} catch {}
	}
	throw new Error("no probe module");
}

export default function (): void {
	const p = probe();
	const targets = [p.planted.absolute, p.planted.relativeJson, "buffer", "node:buffer", "left-pad", "process", p.foreign];
	for (const target of targets) {
		try {
			p.record(`loaded ${target} ${JSON.stringify(load(target))}`);
		} catch (error) {
			p.record(`denied ${(error as Error).message}`);
		}
	}
}
