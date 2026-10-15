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
	check("매개변수 구조 분해·이름 바꾸기·기본값·중첩", describe(row) === "r1:CREATED:9:t0");
	const { id, ...rest } = row;
	check("객체 나머지가 남은 키를 모은다", id === "r1" && Object.keys(rest).join(",") === "kind,meta");
	const [first, , third = "z", ...tail] = ["a", "b", undefined, "d", "e"];
	check("배열 구조 분해·건너뛰기·기본값·나머지", first === "a" && third === "z" && tail.join("") === "de");
	const pairs: string[] = [];
	for (const [key, value] of Object.entries({ x: 1, y: 2 })) pairs.push(`${key}=${value}`);
	check("for-of 안의 배열 구조 분해", pairs.join("&") === "x=1&y=2");
	let left = 1;
	let right = 2;
	[left, right] = [right, left];
	check("구조 분해 대입으로 맞바꾼다", left === 2 && right === 1);
}
