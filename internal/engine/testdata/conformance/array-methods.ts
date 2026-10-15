function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

interface Allowance {
	id: string;
	status: string;
}

const stringValues = (doc: unknown): string[] => {
	if (typeof doc === "string") return [doc];
	if (Array.isArray(doc)) return doc.flatMap(stringValues);
	if (doc !== null && typeof doc === "object") return Object.values(doc).flatMap(stringValues);
	return [];
};

export default function (): void {
	const items: Allowance[] = [
		{ id: "b", status: "BLOCKED" },
		{ id: "a", status: "ALLOWED" },
	];
	check("map·join", items.map((a) => a.id).join("") === "ba");
	check("find·filter", items.find((a) => a.status === "ALLOWED")?.id === "a" && items.filter((a) => a.id > "a").length === 1);
	check("some·every", items.some((a) => a.status === "BLOCKED") && !items.every((a) => a.status === "ALLOWED"));
	check("includes·indexOf", ["x", "y"].includes("y") && ["x", "y"].indexOf("z") === -1);
	check("재귀 flatMap", stringValues({ a: "1", b: ["2", { c: "3" }], d: 4 }).join("") === "123");
	const byLength = ["ccc", "a", "bb"].sort((x, y) => y.length - x.length);
	check("비교 함수 sort", byLength.join(",") === "ccc,bb,a");
	check("인자 없는 sort 는 코드 포인트 순이다", ["b", "a", "B"].sort().join("") === "Bab");
	check("slice", [1, 2, 3, 4].slice(1, 3).join("") === "23" && "abcdef".slice(-3) === "def");
	check("Array.isArray", Array.isArray([]) && !Array.isArray({ length: 0 }));
	check("findIndex·length", items.findIndex((a) => a.id === "a") === 1 && items.length === 2);
}
