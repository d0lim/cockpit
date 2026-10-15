function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const QUERY_MARGIN_MS = 60_000;

export default function (): void {
	check("오프셋 있는 ISO 를 파싱한다", Date.parse("2026-10-15T09:00:00+09:00") === Date.parse("2026-10-15T00:00:00Z"));
	check("toISOString", new Date(Date.UTC(2026, 9, 15, 0, 1, 0) - QUERY_MARGIN_MS).toISOString() === "2026-10-15T00:00:00.000Z");
	check("잘못된 날짜는 NaN 이다", Number.isNaN(Date.parse("not a date")));
	const now = Date.now();
	check("Date.now 는 정수 밀리초다", Number.isInteger(now) && now > Date.UTC(2020, 0, 1));
	check("getTime 왕복", new Date(1_000).getTime() === 1_000);
}
