function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const own = { limit: 20 };
	const inherited = Object.create({ limit: 99 }) as { limit?: number };
	const bare = Object.create(null) as Record<string, unknown>;
	bare.key = 1;
	check("own property is true", Object.hasOwn(own, "limit"));
	check("inherited property is false", !Object.hasOwn(inherited, "limit") && inherited.limit === 99);
	check("works on an object without a prototype", Object.hasOwn(bare, "key") && !Object.hasOwn(bare, "toString"));
}
