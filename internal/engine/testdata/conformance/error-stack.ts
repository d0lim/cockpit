function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

function capture(): string {
	return String(new Error("here").stack);
}

const declared = capture();

export default function (): void {
	const inside = capture();
	check("first stack line is the message", inside.split("\n")[0] === "Error: here");
	check("frame points at the TS file and line", inside.includes("at capture (error-stack.ts:6:"));
	check("caller line is kept too", inside.includes("error-stack.ts:12:"));
	check("module evaluation line is kept too", declared.includes("error-stack.ts:9:"));
}
