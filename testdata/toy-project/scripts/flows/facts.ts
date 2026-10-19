import { type Ctx, expectStatus, flow, json, step } from "cockpit:flow";

export const facts = flow({
	id: "facts",
	title: "The app counts what it saw",
	stage: "observe",
	once: false,
	requires: ["orders"],
	steps: [
		step({
			id: "counters",
			title: "GET /facts counts the created and the refused order",
			kind: "request",
			async run(ctx: Ctx) {
				const res = await ctx.http.send("GET", `${String(ctx.env.appBase)}/facts`);
				expectStatus(res, 200);
				const counts = json<{ created: number; rejected: number }>(res);
				if (counts.created < 1 || counts.rejected < 1) throw new Error(`created ${counts.created}, rejected ${counts.rejected}`);
			},
		}),
	],
});
