import { type CollectCtx, collector } from "cockpit:collect";

export default collector({
	id: "items",
	intervalMs: 5000,
	async run(ctx: CollectCtx) {
		const result = await ctx.sql.query("main", "select id, name, price_cents from items order by id", { maxRows: 100 });
		ctx.signal.metric({ name: "items.rows", value: result.rows.length, attrs: { table: "items", truncated: result.truncated } });
		for (const row of result.rows) {
			ctx.signal.event({ id: `item-${String(row[0])}`, attrs: { table: "items", label: String(row[1]), tone: "info" }, body: JSON.stringify(row) });
		}
	},
});
