import { type Ctx, expectStatus, flow, json, step } from "cockpit:flow";

type Seed = { item: string; quantity: number };

export const orders = flow({
	id: "orders",
	title: "An order is stored and read back",
	stage: "basic",
	once: false,
	requires: ["health"],
	steps: [
		step({
			id: "reject-anonymous",
			title: "An order without a token is refused",
			kind: "request",
			async run(ctx: Ctx) {
				const res = await ctx.http.send("POST", `${String(ctx.env.appBase)}/orders`, { json: { item: "mug", quantity: 1 } });
				expectStatus(res, 401);
			},
		}),
		step({
			id: "create",
			title: "POST /orders with the seed item",
			kind: "request",
			async run(ctx: Ctx) {
				const seed = JSON.parse(ctx.readSource("fixtures", "seed.json")) as Seed;
				const res = await ctx.http.send("POST", `${String(ctx.env.appBase)}/orders`, {
					headers: { authorization: `Bearer ${String(ctx.priv.appToken)}` },
					json: seed,
				});
				expectStatus(res, 201);
				ctx.capture("orderId", json<{ id: string }>(res).id);
			},
		}),
		step({
			id: "read-back",
			title: "GET /orders/{id} returns the seed item",
			kind: "request",
			async run(ctx: Ctx) {
				const res = await ctx.http.send("GET", `${String(ctx.env.appBase)}/orders/${String(ctx.get("orderId"))}`);
				expectStatus(res, 200);
				const order = json<Seed>(res);
				if (order.item !== "kettle" || order.quantity !== 2) throw new Error(`unexpected order ${order.item} x${order.quantity}`);
			},
		}),
	],
});
