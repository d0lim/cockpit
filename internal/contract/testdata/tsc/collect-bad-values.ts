import { collector } from "cockpit:collect";

export default collector({
	id: "values",
	async run(ctx) {
		ctx.signal.event({ attrs: { tone: "purple" } });
		ctx.signal.span({ id: "s-1", traceId: "t-1", name: "주문", start: "2026-10-15T00:00:00.000Z", status: "done" });
	},
});
