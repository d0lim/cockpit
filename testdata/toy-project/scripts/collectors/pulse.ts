import { type CollectCtx, collector } from "cockpit:collect";

export default collector({
	id: "pulse",
	intervalMs: 1000,
	async run(ctx: CollectCtx) {
		const res = await ctx.http.send("GET", "http://app/health");
		ctx.signal.metric({ name: "app.health.status", value: res.status, attrs: { tone: res.status === 200 ? "ok" : "bad" } });
	},
});
