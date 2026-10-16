function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const sameHex = (a: string | null | undefined, b: string): boolean => a?.toLowerCase() === b.toLowerCase();

export default function (): void {
	check("padStart", String(42).padStart(6, "0") === "000042" && "abc".padStart(2, "0") === "abc");
	check("padEnd", "12".padEnd(9, "0") === "120000000");
	check("case conversion", sameHex("0xABCD", "0xabcd") && !sameHex(undefined, "0x"));
	check("startsWith and endsWith", "bearer-token".startsWith("bearer") && "/api/v1/items".endsWith("items"));
	check("includes and indexOf", "a·b·c".includes("·b") && "abc".indexOf("c") === 2);
	check("split and join", "a/b/c".split("/").join("-") === "a-b-c");
	check("slice and substring", "L5-7d".slice(1) === "5-7d" && "abcdef".substring(2, 4) === "cd");
	check("replace changes only the first match", "a.b.c".replace(".", "/") === "a/b.c");
	check("String and toString(16)", String(true) === "true" && (255).toString(16) === "ff");
	check("string comparison", "b" > "a" && "\uac00" > "a");
	check("repeat and length", "ab".repeat(3) === "ababab" && "\ud55c\uae00".length === 2);
}
