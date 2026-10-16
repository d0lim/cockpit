function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

class Client {
	sent = 0;
	lastStatus: number | undefined;
	private readonly limit: number;
	private readonly seen = new Set<string>();
	static created = 0;

	constructor(opts: { limit: number } = { limit: 2 }) {
		this.limit = opts.limit;
		Client.created++;
	}

	send(path: string): boolean {
		if (this.sent >= this.limit) return false;
		this.sent++;
		this.seen.add(path);
		this.lastStatus = 200;
		return true;
	}

	paths(): string[] {
		return [...this.seen];
	}
}

export default function (): void {
	const fresh = new Client();
	check("declared-only field is undefined", fresh.lastStatus === undefined && "lastStatus" in fresh);
	check("field initializer runs per instance", fresh.sent === 0);
	const client = new Client({ limit: 1 });
	check("first send passes", client.send("/a") && client.lastStatus === 200);
	check("send over the limit is blocked", !client.send("/b") && client.sent === 1);
	check("private readonly field exists at runtime", client.paths().join(",") === "/a");
	check("static field grows with each construction", Client.created === 2);
}
