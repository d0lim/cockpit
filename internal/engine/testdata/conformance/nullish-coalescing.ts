function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const zero = 0 as number | null;
	const blank = "" as string | undefined;
	const nothing = null as string | null;
	check("0 is kept", (zero ?? 7) === 0);
	check("empty string is kept", (blank ?? "x") === "");
	check("null is replaced", (nothing ?? "fallback") === "fallback");
	const headers: Record<string, string> = { accept: "*/*" };
	headers["content-type"] ??= "application/json";
	headers.accept ??= "text/plain";
	check("??= fills only missing keys", headers["content-type"] === "application/json" && headers.accept === "*/*");
	const now = (undefined as (() => number) | undefined) ?? (() => 42);
	check("picks a default function", now() === 42);
}
