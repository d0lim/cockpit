function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

interface Record {
	kind: string;
	at: string;
}

export default function (): void {
	const records: Record[] = [
		{ kind: "CREATED", at: "t1" },
		{ kind: "REPORTED", at: "t2" },
	];
	check("음수 인덱스는 끝에서 센다", records.at(-1)?.kind === "REPORTED");
	check("0 은 처음이다", records.at(0)?.at === "t1");
	check("범위 밖은 undefined 다", records.at(5) === undefined && [].at(-1) === undefined);
	check("filter 뒤 at(-1)", records.filter((r) => r.kind === "CREATED").at(-1)?.at === "t1");
	check("문자열 at", "abc".at(-1) === "c");
}
