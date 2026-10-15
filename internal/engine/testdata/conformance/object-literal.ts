function check(label: string, ok: boolean): void {
	if (!ok) throw new Error(label);
}

export default function (): void {
	const url = "http://hook/in";
	const slug = "ws-1";
	const field = "verificationUuid";
	const body = {
		url,
		service: slug,
		"idempotency-key": "k-1",
		[field]: "u-1",
		events: ["A", "B"],
		describe() {
			return `${this.service}@${this.url}`;
		},
	};
	check("단축 속성", body.url === url && body.service === "ws-1");
	check("따옴표 키", body["idempotency-key"] === "k-1");
	check("계산된 키", body.verificationUuid === "u-1");
	check("메서드 단축과 this", body.describe() === "ws-1@http://hook/in");
	check("JSON 에 메서드가 빠진다", JSON.stringify(body).includes(`"events":["A","B"]`) && !JSON.stringify(body).includes("describe"));
}
