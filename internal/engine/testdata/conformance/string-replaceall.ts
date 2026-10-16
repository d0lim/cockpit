function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	check("replaces every match of a string pattern", "a.b.c".replaceAll(".", "/") === "a/b/c");
	check("replaces every match of a g regex", "x1y2".replaceAll(/\d/g, "#") === "x#y#");
	check("replaces with a callback", "ab".replaceAll("b", (m) => m.toUpperCase()) === "aB");
	let threw = false;
	try {
		"aa".replaceAll(/a/, "b");
	} catch (error) {
		threw = error instanceof TypeError;
	}
	check("regex without g throws TypeError", threw);
}
