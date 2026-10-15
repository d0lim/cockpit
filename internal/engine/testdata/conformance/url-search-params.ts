function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const cursor = "c 1";
	const query = new URLSearchParams({ limit: String(20), ...(cursor ? { cursor } : {}) });
	check("객체로 만들고 공백은 + 다", query.toString() === "limit=20&cursor=c+1");
	check("보간이 toString 을 부른다", `/items?${query}` === "/items?limit=20&cursor=c+1");
	const filter: Record<string, string | undefined> = { amount: "5", address: undefined };
	const fromEntries = new URLSearchParams(Object.entries(filter).filter((e): e is [string, string] => typeof e[1] === "string"));
	check("쌍 배열로 만든다", fromEntries.toString() === "amount=5");
	check("비면 빈 문자열이다", new URLSearchParams({}).toString() === "");
	const built = new URLSearchParams();
	built.append("name", "가나");
	built.append("tag", "a&b");
	check("append 가 인코딩한다", built.toString() === "name=%EA%B0%80%EB%82%98&tag=a%26b");
	check("get·has", built.get("tag") === "a&b" && built.has("name") && built.get("none") === null);
	check("문자열을 파싱한다", new URLSearchParams("a=1&b=2").get("b") === "2");
}
