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
	check("as const 배열이 런타임 배열로 남는다", Array.isArray(STEP_KINDS) && STEP_KINDS.includes(kind));
	const def: Def<number> = { id: "d", value: 2 };
	check("제네릭이 지워진다", first([def])?.value === 2);
	const entries: Entry[] = [["a", "x"], ["b", 1]];
	check("타입 술어 필터가 걸러 낸다", entries.filter(isText).length === 1);
	check("satisfies 가 값을 그대로 둔다", config.retries === 3);
	later = "set";
	check("확정 대입 단언이 지워진다", later.length === 3);
	const loose: unknown = null;
	const element = loose as { title?: string } | null;
	check("as 캐스트와 non-null 단언이 지워진다", (element ?? { title: "none" })!.title === "none");
}
