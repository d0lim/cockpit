import { type CollectCtx, collector, type ReservedAttrKey, type SpanStatus, type Tone } from "cockpit:collect";

const keys: ReservedAttrKey[] = ["table", "status", "label", "tone"];
const tones: Tone[] = ["ok", "warn", "bad", "idle", "info"];
const states: SpanStatus[] = ["ok", "error", "incomplete"];

export default collector({
	id: "orders",
	intervalMs: 5000,
	async run(ctx: CollectCtx) {
		const result = await ctx.sql.query("main", "select id, state from orders", { maxRows: 100 });
		for (const row of result.rows) {
			ctx.signal.event({ attrs: { table: "orders", status: String(row[1]), tone: tones[0], label: keys.join(",") }, body: JSON.stringify(row) });
		}
		ctx.signal.metric({ name: "orders.rows", value: result.rows.length, attrs: { table: "orders", truncated: result.truncated } });
		ctx.signal.span({ id: "s-1", traceId: "t-1", name: "주문", start: "2026-10-15T00:00:00.000Z", status: states[2] ?? "ok", tsSource: "container" });
		const logs = await ctx.docker.logs("app", { tail: 10 });
		const out = await ctx.docker.exec("cache", ["valkey-cli", "GET", "k"]);
		const services = await ctx.docker.ps();
		await ctx.state.set("seen", String(logs.text.length + out.stdout.length + services.length));
		const health = await ctx.http.send("GET", "http://app/health");
		if (health.status !== 200) ctx.signal.event({ attrs: { status: "down", tone: "bad" } });
	},
});
