function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

interface Item {
	service?: { slug?: string };
	tags?: string[];
	after?: (value: number) => number;
}

export default function (): void {
	const full: Item = { service: { slug: "alpha" }, tags: ["x"], after: (v) => v + 1 };
	const empty: Item = {};
	const missing: Item | undefined = undefined;
	check("reads a nested property", full.service?.slug === "alpha");
	check("missing intermediate value is undefined", empty.service?.slug === undefined);
	check("missing receiver gives undefined", missing?.service === undefined);
	check("short-circuits computed keys too", full.tags?.[0] === "x" && empty.tags?.[0] === undefined);
	check("optional call invokes the function", full.after?.(1) === 2);
	check("missing function is not called", empty.after?.(1) === undefined);
	let evaluated = false;
	const touch = (): number => {
		evaluated = true;
		return 1;
	};
	void missing?.after?.(touch());
	check("short-circuited chain does not evaluate arguments", !evaluated);
}
