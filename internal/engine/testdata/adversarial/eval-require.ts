interface Probe {
	record(text: string): void;
}

export default function (): void {
	const own = eval("require('cockpit:flow')") as Probe;
	own.record("eval reached cockpit:flow");
	eval("require('cockpit:collect')");
	own.record("foreign module loaded");
}
