function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	check("i 플래그", /^bearer /i.test("Bearer abc"));
	const global = /\d/g;
	const digits: string[] = [];
	let found: RegExpExecArray | null;
	while ((found = global.exec("a1b2")) !== null) digits.push(found[0]);
	check("g 플래그와 lastIndex", digits.join("") === "12" && global.lastIndex === 0);
	check("u 플래그는 아스트랄 문자를 한 글자로 본다", /^.$/u.test("😀") && !/^.$/.test("😀"));
	check("s 플래그는 . 이 줄바꿈을 먹는다", /a.b/s.test("a\nb") && !/a.b/.test("a\nb"));
	const sticky = /foo/y;
	sticky.lastIndex = 3;
	check("y 플래그는 lastIndex 자리에서만 맞춘다", sticky.test("barfoo") && !/foo/y.test("barfoo"));
	const named = /(?<year>\d{4})-(?<month>\d{2})/.exec("2026-10");
	check("이름 있는 그룹", named?.groups?.year === "2026" && named.groups.month === "10");
	check("유니코드 속성 이스케이프", /^\p{Script=Hangul}+$/u.test("한글"));
	check("뒤보기", /(?<=\$)\d+/.exec("cost $42")?.[0] === "42");
}
