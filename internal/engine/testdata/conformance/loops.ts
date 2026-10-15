function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const letters: string[] = [];
	for (const ch of "가a😀") letters.push(ch);
	check("문자열 for-of 는 코드 포인트 단위다", letters.length === 3 && letters[2] === "😀");
	const seen: string[] = [];
	for (const value of new Set(["x", "y"])) seen.push(value);
	for (const [key, value] of new Map([["k", "v"]])) seen.push(`${key}${value}`);
	check("Set·Map 을 돈다", seen.join(",") === "x,y,kv");
	let attempts = 0;
	for (;;) {
		attempts++;
		if (attempts === 3) break;
	}
	check("무한 for 가 break 로 끝난다", attempts === 3);
	const after = ["a", "b", "c", "d"];
	let found = -1;
	for (let i = after.length - 1; i >= 0; i--) {
		if (after[i] === "b") {
			found = i;
			break;
		}
	}
	check("역순 for 가 찾는다", found === 1 && after.slice(found + 1).join("") === "cd");
	const callbacks: (() => number)[] = [];
	for (let i = 0; i < 3; i++) callbacks.push(() => i);
	check("let 은 반복마다 새로 묶인다", callbacks.map((f) => f()).join("") === "012");
	let page = 0;
	while (page < 5) page++;
	check("while 이 돈다", page === 5);
}
