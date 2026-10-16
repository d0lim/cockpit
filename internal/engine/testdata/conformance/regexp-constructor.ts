function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const SECRET_KEY = String.raw`(?:credential|[\w-]*token|password)`;
const QUOTES: readonly string[] = ['"', "&quot;", "%22"];

function keyPattern(): RegExp {
	const q = `(?:${QUOTES.join("|")})`;
	return new RegExp(`(${q}${SECRET_KEY}${q}\\s*(?::|%3A)\\s*${q})[^"\\r\\n<>]{1,64}?(?=${q}|[\\r\\n]|$)`, "gi");
}

export default function (): void {
	const pattern = keyPattern();
	check("constructor takes flags", pattern.flags === "gi" && pattern.global && pattern.ignoreCase);
	const masked = `{"accessToken":"abc123","id":"x"}`.replace(pattern, (_, head: string) => `${head}****`);
	check("lazy quantifier and lookahead", masked === `{"accessToken":"****","id":"x"}`);
	const truncated = `"Password": "open`.replace(keyPattern(), (_, head: string) => `${head}****`);
	check("also stops at the end ($)", truncated === `"Password": "****`);
	check("source returns the pattern", new RegExp("a+b", "y").source === "a+b");
}
