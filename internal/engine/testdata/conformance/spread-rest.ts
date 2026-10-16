function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

function expectOne(actual: number, ...expected: number[]): boolean {
	return expected.includes(actual);
}

const login = (n: number, suffix: string): string[] => [`l${n}-1${suffix}`, `l${n}-2${suffix}`];

export default function (): void {
	const steps = [...login(1, "a"), ...login(2, "b")];
	check("array spread concatenates", steps.join(",") === "l1-1a,l1-2a,l2-1b,l2-2b");
	const base = { surface: "site", loginKey: "k1" };
	const merged = { ...base, id: "s1", loginKey: "k2" };
	check("key after an object spread wins", merged.loginKey === "k2" && merged.surface === "site" && merged.id === "s1");
	const cursor = "" as string;
	const withCursor = (c: string) => ({ limit: "20", ...(c ? { cursor: c } : {}) });
	check("conditional spread adds or omits a key", !("cursor" in withCursor(cursor)) && withCursor("abc").cursor === "abc");
	check("rest parameter is an array", expectOne(201, 200, 201, 409) && !expectOne(500, 200));
	const args: [number, number] = [3, 4];
	check("call spread expands arguments", Math.max(...args) === 4);
	check("spreading a Set removes duplicates", [...new Set(["a", "b", "a"])].join("") === "ab");
	const found: number[] = [];
	found.push(...[1, 2].filter((n) => n > 1));
	check("spread into push", found.length === 1 && found[0] === 2);
}
