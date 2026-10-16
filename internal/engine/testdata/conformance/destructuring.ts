function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

interface Row {
	id: string;
	kind: string;
	extra?: number;
	meta: { at: string };
}

const describe = ({ id, kind: type, extra = 9, meta: { at } }: Row): string => `${id}:${type}:${extra}:${at}`;

export default function (): void {
	const row: Row = { id: "r1", kind: "CREATED", meta: { at: "t0" } };
	check("parameter destructuring with rename, default and nesting", describe(row) === "r1:CREATED:9:t0");
	const { id, ...rest } = row;
	check("object rest collects the remaining keys", id === "r1" && Object.keys(rest).join(",") === "kind,meta");
	const [first, , third = "z", ...tail] = ["a", "b", undefined, "d", "e"];
	check("array destructuring with skip, default and rest", first === "a" && third === "z" && tail.join("") === "de");
	const pairs: string[] = [];
	for (const [key, value] of Object.entries({ x: 1, y: 2 })) pairs.push(`${key}=${value}`);
	check("array destructuring inside for-of", pairs.join("&") === "x=1&y=2");
	let left = 1;
	let right = 2;
	[left, right] = [right, left];
	check("swaps with destructuring assignment", left === 2 && right === 1);
}
