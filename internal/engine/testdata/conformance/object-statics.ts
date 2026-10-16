function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const filter: Record<string, string | undefined> = { amount: "10", address: undefined, kind: "CREATED" };
	const present = Object.entries(filter).filter((e): e is [string, string] => typeof e[1] === "string");
	check("Object.entries yields pairs", present.map(([k, v]) => `${k}=${v}`).join("&") === "amount=10&kind=CREATED");
	check("Object.values", Object.values({ a: 1, b: 2 }).reduce((sum, n) => sum + n, 0) === 3);
	check("Object.keys", Object.keys({ z: 1, y: 2 }).join("") === "zy");
	const rebuilt = Object.fromEntries(present.map(([k, v]) => [k.toUpperCase(), v]));
	check("Object.fromEntries", rebuilt.AMOUNT === "10" && rebuilt.KIND === "CREATED" && !("ADDRESS" in rebuilt));
	const persisted: Record<string, string> = {};
	for (const [name, value] of Object.entries({ session: "s", seed: "t" })) persisted[name] = value;
	check("copies through entries", Object.keys(persisted).length === 2 && persisted.seed === "t");
}
