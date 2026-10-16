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
	check("console methods return undefined", results.every((r) => r === undefined) && results.length === 5);
}
