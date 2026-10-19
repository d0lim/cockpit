import { afterEach, describe, expect, test } from "bun:test";
import { WorkerHub } from "../live/serve";
import { workerLive } from "../live/transport";
import { type Action, initialLive, reduce } from "../store";
import { Sources, settle, sig } from "./fakeSource";
import { OTHER, RUN } from "./fixtures";

const opened: MessagePort[] = [];
afterEach(() => {
	for (const p of opened.splice(0)) p.close();
});

const noTick = () => () => {};

function tab(hub: WorkerHub) {
	const { port1, port2 } = new MessageChannel();
	opened.push(port1, port2);
	hub.connect(port2);
	const live = workerLive(port1, noTick);
	const actions: Action[] = [];
	return { live, actions, push: (a: Action) => actions.push(a) };
}

const signals = (actions: Action[]) => reduce(initialLive(RUN), actions).signals.map((s) => s.seq);

describe("the shared worker", () => {
	test("eight tabs share one connection per target and a tab saying bye leaves the other seven fed", async () => {
		const sources = new Sources();
		const hub = new WorkerHub(sources.open);
		const tabs = Array.from({ length: 8 }, () => tab(hub));
		const offs = tabs.map((t) => t.live.subscribe(RUN, t.push));
		tabs[0]?.live.subscribe(OTHER, () => {});
		tabs[1]?.live.subscribe("hub", () => {});
		await settle(() => sources.live(RUN).length === 1 && sources.live(OTHER).length === 1 && sources.live("hub").length === 1, "three upstreams");
		expect([sources.opened(RUN), sources.opened(OTHER), sources.opened("hub")]).toEqual([1, 1, 1]);
		sources.one(RUN).emit("signal", "e.0.1", sig(1));
		await settle(() => tabs.every((t) => t.actions.length === 1), "first frame everywhere");
		tabs[2]?.live.close();
		offs[3]?.();
		await settle(() => hub.ports() === 7, "the closed tab is forgotten");
		sources.one(RUN).emit("signal", "e.0.2", sig(2));
		await settle(() => signals(tabs[7]?.actions ?? []).length === 2, "second frame");
		expect(tabs.map((t) => signals(t.actions))).toEqual([[1, 2], [1, 2], [1], [1], [1, 2], [1, 2], [1, 2], [1, 2]]);
		expect([sources.opened(RUN), sources.live(RUN).length]).toEqual([1, 1]);
	});

	test("a tab that joins late gets the backlog and a reconnect that resends frames adds none twice", async () => {
		const sources = new Sources();
		const hub = new WorkerHub(sources.open);
		const first = tab(hub);
		first.live.subscribe(RUN, first.push);
		await settle(() => sources.live(RUN).length === 1, "upstream");
		const s = sources.one(RUN);
		s.onopen?.(new Event("open"));
		s.emit("signal", "e.0.1", sig(1));
		s.emit("signal", "e.0.2", sig(2));
		const late = tab(hub);
		late.live.subscribe(RUN, late.push);
		await settle(() => signals(late.actions).length === 2, "late backlog");
		s.onerror?.(new Event("error"));
		s.onopen?.(new Event("open"));
		s.emit("signal", "e.0.2", sig(2));
		s.emit("signal", "e.0.3", sig(3));
		await settle(() => signals(first.actions).length === 3 && signals(late.actions).length === 3, "after reconnect");
		const ids = late.actions.flatMap((a) => (a.kind === "frame" ? [a.id] : []));
		expect(ids).toEqual(["e.0.1", "e.0.2", "e.0.3"]);
		expect(reduce(initialLive(RUN), late.actions).connection).toBe("open");
	});

	test("a tab that stops pinging is swept and its last stream closes", async () => {
		let now = 0;
		const sources = new Sources();
		const hub = new WorkerHub(sources.open, () => now);
		const quiet = tab(hub);
		const alive = tab(hub);
		quiet.live.subscribe(OTHER, quiet.push);
		alive.live.subscribe(RUN, alive.push);
		await settle(() => sources.live(OTHER).length === 1 && sources.live(RUN).length === 1, "two upstreams");
		now = 40_000;
		alive.live.subscribe("hub", alive.push);
		await settle(() => sources.live("hub").length === 1, "a fresh request from the live tab");
		hub.sweep(35_000);
		expect([hub.ports(), sources.live(OTHER).length, sources.live(RUN).length]).toEqual([1, 0, 1]);
	});
});
