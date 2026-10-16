function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const FLOW_ID_PATTERN = /^[a-z][a-z0-9-]{1,40}$/;
const MINIMAL_UNITS = /^[1-9][0-9]*$/;
const INSTANT = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/;

export default function (): void {
	check("anchors and quantifiers", FLOW_ID_PATTERN.test("bootstrap-health") && !FLOW_ID_PATTERN.test("Bad_id"));
	check("minimal-unit integer", MINIMAL_UNITS.test("1000") && !MINIMAL_UNITS.test("0100"));
	const m = INSTANT.exec("2026-10-15T01:02:03.123456Z");
	check("captures and optional group", m !== null && m[1] === "2026-10-15T01:02:03" && m[2] === "123456" && m[3] === "Z");
	const noFraction = INSTANT.exec("2026-10-15T01:02:03+09:00");
	check("missing optional group is undefined", noFraction !== null && noFraction[2] === undefined && noFraction[3] === "+09:00");
	check("no match gives null", INSTANT.exec("yesterday") === null);
	check("replace with the g flag", "AB=CD= EF".replace(/[=\s]/g, "") === "ABCDEF");
	check("split by regex", "a, b,c".split(/,\s*/).join("|") === "a|b|c");
}
