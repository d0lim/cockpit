function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

interface Step {
	id: string;
}

export default function (): void {
	const byName = new Map<string, string>();
	byName.set("b", "2").set("a", "1");
	check("Map 은 넣은 순서를 지킨다", [...byName.keys()].join("") === "ba" && byName.size === 2);
	check("Map get·has·delete", byName.get("a") === "1" && byName.has("b") && byName.delete("b") && !byName.has("b"));
	const persist = new Set<string>(["x"]);
	persist.add("y").add("x");
	persist.delete("y");
	check("Set add·delete·has", persist.size === 1 && persist.has("x"));
	const frames = new WeakMap<Step, string[]>();
	const step = { id: "s1" };
	frames.set(step, ["f1"]);
	check("WeakMap 은 객체 키로 찾는다", frames.get(step)?.[0] === "f1" && frames.get({ id: "s1" }) === undefined);
	const values = [...new Map([["k", 1]]).values()];
	check("반복자를 펼친다", values.length === 1 && values[0] === 1);
	const out: Record<string, string> = {};
	for (const name of new Set(["p", "q"])) out[name] = name.toUpperCase();
	check("Set 을 돌며 객체를 채운다", JSON.stringify(out) === `{"p":"P","q":"Q"}`);
}
