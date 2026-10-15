function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const zero = 0 as number | null;
	const blank = "" as string | undefined;
	const nothing = null as string | null;
	check("0 은 남는다", (zero ?? 7) === 0);
	check("빈 문자열은 남는다", (blank ?? "x") === "");
	check("null 은 대체된다", (nothing ?? "fallback") === "fallback");
	const headers: Record<string, string> = { accept: "*/*" };
	headers["content-type"] ??= "application/json";
	headers.accept ??= "text/plain";
	check("??= 는 빈 자리만 채운다", headers["content-type"] === "application/json" && headers.accept === "*/*");
	const now = (undefined as (() => number) | undefined) ?? (() => 42);
	check("함수 기본값을 고른다", now() === 42);
}
