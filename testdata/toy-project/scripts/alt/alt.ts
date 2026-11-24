import { type Ctx, expectStatus, flow, stage, step, suite } from "cockpit:flow";
import { health } from "../flows/health";

const label = flow({
	id: "env-label",
	title: "The suite reads the public env its declaration names",
	stage: "basic",
	once: false,
	requires: ["health"],
	steps: [
		step({
			id: "label-alt",
			title: "env.label is alt and the app answers at env.appBase",
			kind: "request",
			async run(ctx: Ctx) {
				if (ctx.env.label !== "alt") throw new Error(`env.label is ${String(ctx.env.label)}, want alt`);
				const res = await ctx.http.send("GET", `${String(ctx.env.appBase)}/health`);
				expectStatus(res, 200);
			},
		}),
	],
});

export default suite({
	name: "alt",
	stages: [stage({ id: "basic", title: "Basic" })],
	flows: [health, label],
});
