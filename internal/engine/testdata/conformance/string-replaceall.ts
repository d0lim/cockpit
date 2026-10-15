function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	check("문자열 패턴을 모두 바꾼다", "a.b.c".replaceAll(".", "/") === "a/b/c");
	check("g 정규식을 모두 바꾼다", "x1y2".replaceAll(/\d/g, "#") === "x#y#");
	check("콜백으로 바꾼다", "ab".replaceAll("b", (m) => m.toUpperCase()) === "aB");
	let threw = false;
	try {
		"aa".replaceAll(/a/, "b");
	} catch (error) {
		threw = error instanceof TypeError;
	}
	check("g 없는 정규식은 TypeError 다", threw);
}
