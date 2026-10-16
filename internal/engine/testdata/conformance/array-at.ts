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
	check("negative index counts from the end", records.at(-1)?.kind === "REPORTED");
	check("index 0 is the first item", records.at(0)?.at === "t1");
	check("out of range is undefined", records.at(5) === undefined && [].at(-1) === undefined);
	check("at(-1) after filter", records.filter((r) => r.kind === "CREATED").at(-1)?.at === "t1");
	check("string at", "abc".at(-1) === "c");
}
