function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

interface Item {
	service?: { slug?: string };
	tags?: string[];
	after?: (value: number) => number;
}

export default function (): void {
	const full: Item = { service: { slug: "alpha" }, tags: ["x"], after: (v) => v + 1 };
	const empty: Item = {};
	const missing: Item | undefined = undefined;
	check("중첩 속성을 읽는다", full.service?.slug === "alpha");
	check("빈 중간 값은 undefined 다", empty.service?.slug === undefined);
	check("수신자가 없으면 undefined 다", missing?.service === undefined);
	check("계산된 키도 끊는다", full.tags?.[0] === "x" && empty.tags?.[0] === undefined);
	check("선택 호출이 부른다", full.after?.(1) === 2);
	check("없는 함수는 부르지 않는다", empty.after?.(1) === undefined);
	let evaluated = false;
	const touch = (): number => {
		evaluated = true;
		return 1;
	};
	void missing?.after?.(touch());
	check("끊긴 체인은 인자를 평가하지 않는다", !evaluated);
}
