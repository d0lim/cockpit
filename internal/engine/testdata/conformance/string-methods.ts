function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const sameHex = (a: string | null | undefined, b: string): boolean => a?.toLowerCase() === b.toLowerCase();

export default function (): void {
	check("padStart", String(42).padStart(6, "0") === "000042" && "abc".padStart(2, "0") === "abc");
	check("padEnd", "12".padEnd(9, "0") === "120000000");
	check("대소문자", sameHex("0xABCD", "0xabcd") && !sameHex(undefined, "0x"));
	check("startsWith·endsWith", "bearer-token".startsWith("bearer") && "/api/v1/items".endsWith("items"));
	check("includes·indexOf", "a·b·c".includes("·b") && "abc".indexOf("c") === 2);
	check("split·join", "a/b/c".split("/").join("-") === "a-b-c");
	check("slice·substring", "L5-7d".slice(1) === "5-7d" && "abcdef".substring(2, 4) === "cd");
	check("replace 는 첫 번째만 바꾼다", "a.b.c".replace(".", "/") === "a/b.c");
	check("String 과 toString(16)", String(true) === "true" && (255).toString(16) === "ff");
	check("문자열 비교", "b" > "a" && "가" > "a");
	check("repeat·length", "ab".repeat(3) === "ababab" && "한글".length === 2);
}
