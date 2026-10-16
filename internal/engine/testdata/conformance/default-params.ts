function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const COUNTERPARTY = "peer-default";

function accept(prefix: string, amount: string, counterparty = COUNTERPARTY): string {
	return `${prefix}:${amount}:${counterparty}`;
}

function records(filter: Record<string, string> = {}): number {
	return Object.keys(filter).length;
}

function issue(code: string, digits = 6, pad = "0".repeat(digits)): string {
	return (pad + code).slice(-digits);
}

export default function (): void {
	check("default fills a missing argument", accept("w", "1") === "w:1:peer-default");
	check("explicit argument wins", accept("w", "1", "other") === "w:1:other");
	check("undefined also triggers the default", accept("w", "1", undefined) === "w:1:peer-default");
	check("object default is created per call", records() === 0 && records({ a: "1" }) === 1);
	check("default uses an earlier parameter", issue("42") === "000042" && issue("7", 3) === "007");
}
