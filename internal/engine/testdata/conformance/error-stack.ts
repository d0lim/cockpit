function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

function capture(): string {
	return String(new Error("here").stack);
}

const declared = capture();

export default function (): void {
	const inside = capture();
	check("스택 첫 줄은 메시지다", inside.split("\n")[0] === "Error: here");
	check("프레임이 TS 파일과 줄을 가리킨다", inside.includes("at capture (error-stack.ts:6:"));
	check("부른 자리의 줄도 남는다", inside.includes("error-stack.ts:12:"));
	check("모듈 평가 자리의 줄도 남는다", declared.includes("error-stack.ts:9:"));
}
