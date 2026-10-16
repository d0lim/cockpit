function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const cursor = "c 1";
	const query = new URLSearchParams({ limit: String(20), ...(cursor ? { cursor } : {}) });
	check("built from an object, space becomes +", query.toString() === "limit=20&cursor=c+1");
	check("interpolation calls toString", `/items?${query}` === "/items?limit=20&cursor=c+1");
	const filter: Record<string, string | undefined> = { amount: "5", address: undefined };
	const fromEntries = new URLSearchParams(Object.entries(filter).filter((e): e is [string, string] => typeof e[1] === "string"));
	check("built from an array of pairs", fromEntries.toString() === "amount=5");
	check("empty params give an empty string", new URLSearchParams({}).toString() === "");
	const built = new URLSearchParams();
	built.append("name", "\uac00\ub098");
	built.append("tag", "a&b");
	check("append encodes values", built.toString() === "name=%EA%B0%80%EB%82%98&tag=a%26b");
	check("get and has", built.get("tag") === "a&b" && built.has("name") && built.get("none") === null);
	check("parses a query string", new URLSearchParams("a=1&b=2").get("b") === "2");
}
