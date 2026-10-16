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
	check("shorthand property", body.url === url && body.service === "ws-1");
	check("quoted key", body["idempotency-key"] === "k-1");
	check("computed key", body.verificationUuid === "u-1");
	check("method shorthand and this", body.describe() === "ws-1@http://hook/in");
	check("JSON omits methods", JSON.stringify(body).includes(`"events":["A","B"]`) && !JSON.stringify(body).includes("describe"));
}
