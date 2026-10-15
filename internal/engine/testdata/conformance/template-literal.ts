function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const method = "GET";
	const status = 404;
	const line = `${method} /items — 상태 ${status}`;
	check("보간이 값을 넣는다", line === "GET /items — 상태 404");
	const nested = `outer ${`inner ${status + 1}`} end`;
	check("중첩 템플릿이 풀린다", nested === "outer inner 405 end");
	const multi = `a
b`;
	check("줄바꿈이 남는다", multi === "a\nb");
	const raw = String.raw`(?:token|secret)\s*\d`;
	check("String.raw 는 역슬래시를 남긴다", raw === "(?:token|secret)\\s*\\d" && raw.length === 21);
	const joined = `${["a", "b"].join("·")}`;
	check("식 결과가 문자열이 된다", joined === "a·b");
}
