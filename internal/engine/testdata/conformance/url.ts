function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

function target(method: string, url: string): string {
	try {
		return `${method} ${new URL(url).pathname}`;
	} catch {
		return method;
	}
}

export default function (): void {
	const url = new URL("http://svc:8080/api/v1/items?limit=5#top");
	check("pathname", url.pathname === "/api/v1/items");
	check("host·port·protocol", url.host === "svc:8080" && url.port === "8080" && url.protocol === "http:");
	check("searchParams", url.searchParams.get("limit") === "5" && url.search === "?limit=5");
	check("쿼리 없이 경로만 쓴다", target("GET", "http://svc/a/b?token=x") === "GET /a/b");
	check("잘못된 URL 은 던진다", target("POST", "::not a url::") === "POST");
	check("점 경로를 접는다", new URL("http://svc/a/b/../c").pathname === "/a/c");
}
