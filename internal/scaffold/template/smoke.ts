import { expectStatus, flow, stage, step, suite } from "cockpit:flow";

const home = flow({
	id: "home",
	title: "Home page",
	stage: "basic",
	once: false,
	requires: [],
	steps: [
		step({
			id: "get",
			title: "GET / answers 200",
			kind: "request",
			async run(ctx) {
				const res = await ctx.http.send("GET", `${ctx.env.baseUrl}/`);
				expectStatus(res, 200);
				ctx.capture("home.etag", res.headers["etag"] ?? "none");
			},
		}),
	],
});

const missing = flow({
	id: "missing",
	title: "Unknown path",
	stage: "basic",
	once: false,
	requires: ["home"],
	steps: [
		step({
			id: "get",
			title: "GET /nope answers 404",
			kind: "request",
			async run(ctx) {
				const res = await ctx.http.send("GET", `${ctx.env.baseUrl}/nope`, {
					headers: { "if-none-match": ctx.get("home.etag") },
				});
				expectStatus(res, 404);
			},
		}),
	],
});

export default suite({
	name: "smoke",
	stages: [stage({ id: "basic", title: "Basic" })],
	flows: [home, missing],
});
