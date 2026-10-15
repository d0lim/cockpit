function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const doc = { schemaVersion: 1, captures: { 이름: "값" }, list: [1, "two", null, true], nested: { deep: { n: 1.5 } } };
	const text = `${JSON.stringify(doc, null, 2)}\n`;
	check("들여쓰기 2 와 끝 줄바꿈", text.startsWith('{\n  "schemaVersion": 1,') && text.endsWith("}\n"));
	const back = JSON.parse(text) as typeof doc;
	check("왕복이 값을 지킨다", back.captures.이름 === "값" && back.list[3] === true && back.nested.deep.n === 1.5);
	check("한 줄 직렬화", JSON.stringify({ a: [1, { b: "c" }] }) === '{"a":[1,{"b":"c"}]}');
	check("undefined 는 빠진다", JSON.stringify({ a: undefined, b: 1 }) === '{"b":1}');
	let threw = "";
	try {
		JSON.parse("{bad");
	} catch (error) {
		threw = error instanceof SyntaxError ? "syntax" : "other";
	}
	check("잘못된 JSON 은 SyntaxError 다", threw === "syntax");
	check("문자열 이스케이프", JSON.parse(JSON.stringify('a"b\\c\n')) === 'a"b\\c\n');
}
