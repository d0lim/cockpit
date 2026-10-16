function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const method = "GET";
	const status = 404;
	const line = `${method} /items — status ${status}`;
	check("interpolation inserts values", line === "GET /items — status 404");
	const nested = `outer ${`inner ${status + 1}`} end`;
	check("nested template resolves", nested === "outer inner 405 end");
	const multi = `a
b`;
	check("newline is kept", multi === "a\nb");
	const raw = String.raw`(?:token|secret)\s*\d`;
	check("String.raw keeps backslashes", raw === "(?:token|secret)\\s*\\d" && raw.length === 21);
	const joined = `${["a", "b"].join("·")}`;
	check("expression result becomes a string", joined === "a·b");
}
