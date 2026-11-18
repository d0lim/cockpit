import { type Ctx, expectStatus, flow, json, step } from "cockpit:flow";

const app = (ctx: Ctx) => String(ctx.env.appBase);

function relayStep(id: string, trace: string) {
	return step({
		id,
		title: `POST /relay with trace ${trace}`,
		kind: "request",
		async run(ctx: Ctx) {
			const res = await ctx.http.send("POST", `${app(ctx)}/relay?trace=${trace}`, { json: { from: "runner" } });
			expectStatus(res, 200);
			if (!json<{ ledger: { ok: boolean } }>(res).ledger.ok) throw new Error("the ledger did not confirm the entry");
		},
	});
}

function getStep(id: string, path: string) {
	return step({
		id,
		title: `GET ${path}`,
		kind: "request",
		async run(ctx: Ctx) {
			expectStatus(await ctx.http.send("GET", `${app(ctx)}${path}`), 200);
		},
	});
}

export const up = flow({
	id: "up",
	title: "Both services answer",
	stage: "calls",
	once: false,
	requires: [],
	steps: [
		getStep("app-health", "/health"),
		step({
			id: "ledger-health",
			title: "GET /health on the ledger",
			kind: "request",
			async run(ctx: Ctx) {
				expectStatus(await ctx.http.send("GET", `${String(ctx.env.ledgerBase)}/health`), 200);
			},
		}),
	],
});

export const relay = flow({
	id: "relay",
	title: "The app calls the ledger while it answers",
	stage: "calls",
	once: false,
	requires: ["up"],
	steps: [relayStep("child-span", "child"), relayStep("no-span", "none"), relayStep("same-span", "same")],
});

export const timing = flow({
	id: "timing",
	title: "A call that starts after its cause has returned",
	stage: "calls",
	once: false,
	requires: ["up"],
	steps: [
		step({
			id: "schedule",
			title: "POST /later schedules a hook",
			kind: "request",
			async run(ctx: Ctx) {
				expectStatus(await ctx.http.send("POST", `${app(ctx)}/later?delayMs=700&hook=late`, { json: {} }), 202);
			},
		}),
		step({
			id: "hook-window",
			title: "The hook arrives while this step waits",
			kind: "wait",
			async run(ctx: Ctx) {
				await ctx.sleep(2500);
			},
		}),
	],
});

export const callback = flow({
	id: "callback",
	title: "A service dials an address the flow handed it",
	stage: "calls",
	once: false,
	requires: ["up"],
	steps: [
		step({
			id: "register",
			title: "POST /register with the callback from env.json",
			kind: "request",
			async run(ctx: Ctx) {
				const res = await ctx.http.send("POST", `${app(ctx)}/register`, { json: { callback: String(ctx.env.ledgerCallback) } });
				expectStatus(res, 200);
			},
		}),
	],
});

export const bodies = flow({
	id: "bodies",
	title: "Bodies of every kind",
	stage: "bodies",
	once: false,
	requires: ["up"],
	steps: [getStep("jwt", "/jwt"), getStep("pem-begin", "/pem/begin"), getStep("pem-end", "/pem/end"), getStep("big", "/big"), getStep("binary", "/binary")],
});

export const closing = flow({
	id: "closing",
	title: "A call that starts after the last step",
	stage: "end",
	once: false,
	requires: ["up"],
	steps: [
		step({
			id: "schedule-after",
			title: "POST /later schedules a hook past the end of the suite",
			kind: "request",
			async run(ctx: Ctx) {
				expectStatus(await ctx.http.send("POST", `${app(ctx)}/later?delayMs=800&hook=after`, { json: {} }), 202);
			},
		}),
	],
});

export const hold = flow({
	id: "hold",
	title: "A call that never ends",
	stage: "calls",
	once: false,
	requires: [],
	steps: [
		step({
			id: "held",
			title: "POST /slow stays open until the run is aborted",
			kind: "request",
			async run(ctx: Ctx) {
				await ctx.http.send("POST", `${app(ctx)}/slow`, { json: {}, timeoutMs: 240_000 });
			},
		}),
	],
});
