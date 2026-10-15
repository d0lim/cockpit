function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const results = [
		console.log("log", 1, { a: true }),
		console.info("info"),
		console.warn("warn %s", "fmt"),
		console.error("error"),
		console.debug("debug"),
	];
	check("console 메서드는 undefined 를 돌려준다", results.every((r) => r === undefined) && results.length === 5);
}
