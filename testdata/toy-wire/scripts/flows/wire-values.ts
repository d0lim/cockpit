import { type Ctx, expectStatus, flow, json, step } from "cockpit:flow";

const app = (ctx: Ctx) => String(ctx.env.appBase);

function echoStep(id: string, value: (ctx: Ctx) => string) {
	return step({
		id,
		title: "POST /echo carries a credential in every place",
		kind: "request",
		async run(ctx: Ctx) {
			const secret = value(ctx);
			const res = await ctx.http.send("POST", `${app(ctx)}/echo?note=${encodeURIComponent(secret)}`, {
				headers: { authorization: `Bearer ${secret}`, "x-partner-key": secret },
				json: { note: secret },
			});
			expectStatus(res, 200);
			if (json<{ note: string }>(res).note !== secret) throw new Error("the echo differs");
		},
	});
}

export const credentials = flow({
	id: "credentials",
	title: "Credentials in queries, headers and bodies",
	stage: "values",
	once: false,
	requires: ["up"],
	steps: [
		echoStep("env-token", (ctx) => String(ctx.priv.appToken)),
		echoStep("private-key", (ctx) => String(ctx.priv.partnerKey)),
		step({
			id: "path-secret",
			title: "GET /accounts/{key} carries a credential in the path",
			kind: "request",
			async run(ctx: Ctx) {
				expectStatus(await ctx.http.send("GET", `${app(ctx)}/accounts/${String(ctx.priv.partnerKey)}`), 200);
			},
		}),
		step({
			id: "service-secret",
			title: "POST /secret-relay sends the database password to the ledger",
			kind: "request",
			async run(ctx: Ctx) {
				expectStatus(await ctx.http.send("POST", `${app(ctx)}/secret-relay`, { json: {} }), 200);
			},
		}),
		step({
			id: "shapes",
			title: "GET /shapes returns public and private fields",
			kind: "request",
			async run(ctx: Ctx) {
				expectStatus(await ctx.http.send("GET", `${app(ctx)}/shapes`), 200);
			},
		}),
	],
});

export const learned = flow({
	id: "learned",
	title: "Values the flow learns from responses",
	stage: "values",
	once: false,
	requires: ["up"],
	steps: [
		step({
			id: "issue",
			title: "POST /tickets returns a reference and a public id",
			kind: "request",
			async run(ctx: Ctx) {
				const res = await ctx.http.send("POST", `${app(ctx)}/tickets`, { json: {} });
				expectStatus(res, 201);
				const issued = json<{ id: string; reference: string }>(res);
				ctx.capture("reference", issued.reference);
				ctx.capture("ticketId", issued.id);
				ctx.ref("ticket", issued.id);
			},
		}),
		step({
			id: "redeem",
			title: "POST /tickets/redeem sends both back",
			kind: "request",
			async run(ctx: Ctx) {
				const res = await ctx.http.send("POST", `${app(ctx)}/tickets/redeem`, { json: { id: ctx.get("ticketId"), reference: ctx.get("reference") } });
				expectStatus(res, 200);
			},
		}),
	],
});

export const recall = flow({
	id: "recall",
	title: "A service repeats a value an earlier flow captured",
	stage: "values",
	once: false,
	requires: ["up"],
	steps: [
		step({
			id: "latest",
			title: "GET /tickets/latest returns the last reference",
			kind: "request",
			async run(ctx: Ctx) {
				expectStatus(await ctx.http.send("GET", `${app(ctx)}/tickets/latest`), 200);
			},
		}),
	],
});
