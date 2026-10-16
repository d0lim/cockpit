function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

const STEP_KINDS = ["request", "wait", "capture"] as const;
type StepKind = (typeof STEP_KINDS)[number];

interface Def<T> {
	id: string;
	value: T;
}

type Entry = [string, string | number];

const first = <T>(items: readonly T[]): T | undefined => items[0];

const isText = (entry: Entry): entry is [string, string] => typeof entry[1] === "string";

const config = { retries: 3 } satisfies Record<string, number>;

let later!: string;

export default function (): void {
	const kind: StepKind = "wait";
	check("as const array stays a runtime array", Array.isArray(STEP_KINDS) && STEP_KINDS.includes(kind));
	const def: Def<number> = { id: "d", value: 2 };
	check("generics are erased", first([def])?.value === 2);
	const entries: Entry[] = [["a", "x"], ["b", 1]];
	check("type predicate filter narrows", entries.filter(isText).length === 1);
	check("satisfies leaves the value unchanged", config.retries === 3);
	later = "set";
	check("definite assignment assertion is erased", later.length === 3);
	const loose: unknown = null;
	const element = loose as { title?: string } | null;
	check("as cast and non-null assertion are erased", (element ?? { title: "none" })!.title === "none");
}
