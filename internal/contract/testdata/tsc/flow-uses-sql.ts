import { flow, step } from "cockpit:flow";

export default flow({
	id: "sneaky",
	title: "Reads the DB in the runner",
	stage: "basic",
	once: false,
	requires: [],
	steps: [
		step({
			id: "read",
			title: "Reads the orders table",
			kind: "request",
			async run(ctx) {
				await ctx.sql.query("main", "select 1");
			},
		}),
	],
});
