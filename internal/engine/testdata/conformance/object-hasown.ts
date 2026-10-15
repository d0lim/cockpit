function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const own = { limit: 20 };
	const inherited = Object.create({ limit: 99 }) as { limit?: number };
	const bare = Object.create(null) as Record<string, unknown>;
	bare.key = 1;
	check("자기 속성은 참이다", Object.hasOwn(own, "limit"));
	check("상속된 속성은 거짓이다", !Object.hasOwn(inherited, "limit") && inherited.limit === 99);
	check("프로토타입 없는 객체에도 쓴다", Object.hasOwn(bare, "key") && !Object.hasOwn(bare, "toString"));
}
