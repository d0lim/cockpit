import { flow, step } from "cockpit:flow";

export default flow({
	id: "sneaky",
	title: "러너 자리에서 DB 를 읽는다",
	stage: "basic",
	once: false,
	requires: [],
	steps: [
		step({
			id: "read",
			title: "주문 표를 읽는다",
			kind: "request",
			async run(ctx) {
				await ctx.sql.query("main", "select 1");
			},
		}),
	],
});
