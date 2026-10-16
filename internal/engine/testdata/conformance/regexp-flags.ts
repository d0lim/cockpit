function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	check("i flag", /^bearer /i.test("Bearer abc"));
	const global = /\d/g;
	const digits: string[] = [];
	let found: RegExpExecArray | null;
	while ((found = global.exec("a1b2")) !== null) digits.push(found[0]);
	check("g flag and lastIndex", digits.join("") === "12" && global.lastIndex === 0);
	check("u flag treats an astral character as one character", /^.$/u.test("😀") && !/^.$/.test("😀"));
	check("s flag lets . match a newline", /a.b/s.test("a\nb") && !/a.b/.test("a\nb"));
	const sticky = /foo/y;
	sticky.lastIndex = 3;
	check("y flag matches only at lastIndex", sticky.test("barfoo") && !/foo/y.test("barfoo"));
	const named = /(?<year>\d{4})-(?<month>\d{2})/.exec("2026-10");
	check("named groups", named?.groups?.year === "2026" && named.groups.month === "10");
	check("Unicode property escape", /^\p{Script=Hangul}+$/u.test("\ud55c\uae00"));
	check("lookbehind", /(?<=\$)\d+/.exec("cost $42")?.[0] === "42");
}
