import { type CollectCtx, collector } from "cockpit:collect";

export default collector({
	id: "cache",
	intervalMs: 5000,
	async run(ctx: CollectCtx) {
		const out = await ctx.docker.exec("cache", ["valkey-cli", "DBSIZE"]);
		const keys = Number(out.stdout.trim());
		ctx.signal.metric({ name: "cache.keys", value: Number.isFinite(keys) ? keys : -1, attrs: { tone: Number.isFinite(keys) ? "ok" : "bad" } });
		const seq = await ctx.docker.exec("cache", ["valkey-cli", "GET", "order:seq"]);
		await ctx.state.set("lastSeq", seq.stdout.trim());
	},
});
