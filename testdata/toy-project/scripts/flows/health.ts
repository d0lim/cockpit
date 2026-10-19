import { type Ctx, expectStatus, flow, json, poll, step } from "cockpit:flow";

export const health = flow({
	id: "health",
	title: "App answers its health check",
	stage: "basic",
	once: false,
	requires: [],
	steps: [
		step({
			id: "health-ok",
			title: "GET /health returns ok",
			kind: "wait",
			async run(ctx: Ctx) {
				const base = String(ctx.env.appBase);
				const { value } = await poll(
					async () => {
						const res = await ctx.http.send("GET", `${base}/health`);
						expectStatus(res, 200);
						return json<{ ok: boolean }>(res).ok;
					},
					(ok) => ok,
					{ what: "app health", deadlineMs: 30_000, intervalMs: 500, now: ctx.clock, sleep: ctx.sleep },
				);
				if (!value) throw new Error("app health never became ok");
			},
		}),
	],
});
