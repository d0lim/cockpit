function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const letters: string[] = [];
	for (const ch of "\uac00a😀") letters.push(ch);
	check("string for-of iterates by code point", letters.length === 3 && letters[2] === "😀");
	const seen: string[] = [];
	for (const value of new Set(["x", "y"])) seen.push(value);
	for (const [key, value] of new Map([["k", "v"]])) seen.push(`${key}${value}`);
	check("iterates a Set and a Map", seen.join(",") === "x,y,kv");
	let attempts = 0;
	for (;;) {
		attempts++;
		if (attempts === 3) break;
	}
	check("infinite for ends with break", attempts === 3);
	const after = ["a", "b", "c", "d"];
	let found = -1;
	for (let i = after.length - 1; i >= 0; i--) {
		if (after[i] === "b") {
			found = i;
			break;
		}
	}
	check("reverse for finds the item", found === 1 && after.slice(found + 1).join("") === "cd");
	const callbacks: (() => number)[] = [];
	for (let i = 0; i < 3; i++) callbacks.push(() => i);
	check("let is rebound on each iteration", callbacks.map((f) => f()).join("") === "012");
	let page = 0;
	while (page < 5) page++;
	check("while loops", page === 5);
}
