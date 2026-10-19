import { type CollectCtx, collector } from "cockpit:collect";

export default collector({
	id: "facts",
	async run(ctx: CollectCtx) {
		const res = await ctx.http.send("GET", "http://app:8080/facts");
		if (res.status !== 200) {
			ctx.signal.event({ attrs: { status: String(res.status), tone: "bad", label: "facts unavailable" } });
			return;
		}
		const counts = JSON.parse(res.body) as { created: number; rejected: number };
		ctx.signal.metric({ name: "app.orders.created", value: counts.created });
		ctx.signal.metric({ name: "app.orders.rejected", value: counts.rejected });
	},
});
