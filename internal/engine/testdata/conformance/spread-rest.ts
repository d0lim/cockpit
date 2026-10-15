function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

function expectOne(actual: number, ...expected: number[]): boolean {
	return expected.includes(actual);
}

const login = (n: number, suffix: string): string[] => [`l${n}-1${suffix}`, `l${n}-2${suffix}`];

export default function (): void {
	const steps = [...login(1, "a"), ...login(2, "b")];
	check("배열 전개가 이어 붙인다", steps.join(",") === "l1-1a,l1-2a,l2-1b,l2-2b");
	const base = { surface: "site", loginKey: "k1" };
	const merged = { ...base, id: "s1", loginKey: "k2" };
	check("객체 전개 뒤의 키가 이긴다", merged.loginKey === "k2" && merged.surface === "site" && merged.id === "s1");
	const cursor = "" as string;
	const withCursor = (c: string) => ({ limit: "20", ...(c ? { cursor: c } : {}) });
	check("조건부 전개가 키를 넣고 뺀다", !("cursor" in withCursor(cursor)) && withCursor("abc").cursor === "abc");
	check("나머지 매개변수가 배열이다", expectOne(201, 200, 201, 409) && !expectOne(500, 200));
	const args: [number, number] = [3, 4];
	check("호출 전개가 인자를 푼다", Math.max(...args) === 4);
	check("Set 을 펼쳐 중복을 뺀다", [...new Set(["a", "b", "a"])].join("") === "ab");
	const found: number[] = [];
	found.push(...[1, 2].filter((n) => n > 1));
	check("push 에 전개를 넘긴다", found.length === 1 && found[0] === 2);
}
