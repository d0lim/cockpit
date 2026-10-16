import { type Ctx, expectStatus, flow, stage, step, suite } from "cockpit:flow";

const bad = step({
	id: "bad",
	title: "Wrong status type",
	kind: "request",
	async run(ctx: Ctx) {
		const res = await ctx.http.send("GET", String(ctx.env.appBase));
		expectStatus(res, "200");
	},
});

export default suite({ name: "bad", stages: [stage({ id: "basic", title: "Basic" })], flows: [flow({ id: "bad", title: "Bad", stage: "basic", once: false, requires: [], steps: [bad] })] });
