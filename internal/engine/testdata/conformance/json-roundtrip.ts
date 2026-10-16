function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const doc = { schemaVersion: 1, captures: { \uc774\ub984: "\uac12" }, list: [1, "two", null, true], nested: { deep: { n: 1.5 } } };
	const text = `${JSON.stringify(doc, null, 2)}\n`;
	check("indent of 2 and trailing newline", text.startsWith('{\n  "schemaVersion": 1,') && text.endsWith("}\n"));
	const back = JSON.parse(text) as typeof doc;
	check("round trip keeps values, non-ASCII key included", back.captures.\uc774\ub984 === "\uac12" && back.list[3] === true && back.nested.deep.n === 1.5);
	check("single-line serialization", JSON.stringify({ a: [1, { b: "c" }] }) === '{"a":[1,{"b":"c"}]}');
	check("undefined is dropped", JSON.stringify({ a: undefined, b: 1 }) === '{"b":1}');
	let threw = "";
	try {
		JSON.parse("{bad");
	} catch (error) {
		threw = error instanceof SyntaxError ? "syntax" : "other";
	}
	check("invalid JSON is a SyntaxError", threw === "syntax");
	check("string escapes", JSON.parse(JSON.stringify('a"b\\c\n')) === 'a"b\\c\n');
}
