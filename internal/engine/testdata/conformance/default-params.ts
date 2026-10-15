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
	check("기본값이 빈 인자를 채운다", accept("w", "1") === "w:1:peer-default");
	check("명시한 인자가 이긴다", accept("w", "1", "other") === "w:1:other");
	check("undefined 도 기본값을 부른다", accept("w", "1", undefined) === "w:1:peer-default");
	check("객체 기본값은 호출마다 새로 만든다", records() === 0 && records({ a: "1" }) === 1);
	check("앞 매개변수를 기본값이 쓴다", issue("42") === "000042" && issue("7", 3) === "007");
}
