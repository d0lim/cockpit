import { type Ctx, type FlowDef, flow, stage, step, suite } from "cockpit:flow";

const ping = step({
	id: "ping",
	title: "앱이 응답한다",
	kind: "request",
	async run(ctx: Ctx) {
		const res = await ctx.http.send("GET", `${String(ctx.env.appBase)}/health`, { headers: { accept: "application/json" } });
		if (res.status !== 200 || res.headers.get("content-type") === null) throw new Error(`상태 ${res.status}`);
		ctx.capture("requestId", ctx.crypto.randomUUID(), { persist: true });
		const seed = ctx.readSource("fixtures", "seed.json");
		const names = ctx.listSource("fixtures", ".");
		const code = await ctx.totp(ctx.get("requestId"));
		ctx.saveSeed("user-1", `${seed.length}-${names.length}-${code}-${ctx.priv.token ?? ""}`);
		const bytes = ctx.crypto.getRandomValues(new Uint8Array(4));
		await ctx.sleep((ctx.clock() + bytes.length) % 10);
	},
});

const smoke: FlowDef = flow({ id: "smoke", title: "스모크", stage: "basic", once: false, requires: [], steps: [ping] });

export default suite({ name: "smoke", stages: [stage({ id: "basic", title: "기본" })], flows: [smoke] });
