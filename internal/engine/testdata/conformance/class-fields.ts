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
	check("선언만 한 필드는 undefined 다", fresh.lastStatus === undefined && "lastStatus" in fresh);
	check("필드 초기값이 인스턴스마다 선다", fresh.sent === 0);
	const client = new Client({ limit: 1 });
	check("첫 전송은 통과한다", client.send("/a") && client.lastStatus === 200);
	check("상한을 넘기면 막힌다", !client.send("/b") && client.sent === 1);
	check("private readonly 필드가 런타임에 남는다", client.paths().join(",") === "/a");
	check("정적 필드가 생성마다 는다", Client.created === 2);
}
