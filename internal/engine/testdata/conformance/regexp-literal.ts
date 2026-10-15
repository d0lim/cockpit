function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const FLOW_ID_PATTERN = /^[a-z][a-z0-9-]{1,40}$/;
const MINIMAL_UNITS = /^[1-9][0-9]*$/;
const INSTANT = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/;

export default function (): void {
	check("앵커와 수량자", FLOW_ID_PATTERN.test("bootstrap-health") && !FLOW_ID_PATTERN.test("Bad_id"));
	check("최소 단위 정수", MINIMAL_UNITS.test("1000") && !MINIMAL_UNITS.test("0100"));
	const m = INSTANT.exec("2026-10-15T01:02:03.123456Z");
	check("캡처와 선택 그룹", m !== null && m[1] === "2026-10-15T01:02:03" && m[2] === "123456" && m[3] === "Z");
	const noFraction = INSTANT.exec("2026-10-15T01:02:03+09:00");
	check("빠진 선택 그룹은 undefined 다", noFraction !== null && noFraction[2] === undefined && noFraction[3] === "+09:00");
	check("맞지 않으면 null 이다", INSTANT.exec("yesterday") === null);
	check("g 플래그 replace", "AB=CD= EF".replace(/[=\s]/g, "") === "ABCDEF");
	check("정규식 split", "a, b,c".split(/,\s*/).join("|") === "a|b|c");
}
