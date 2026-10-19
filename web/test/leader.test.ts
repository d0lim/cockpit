import { describe, expect, test } from "bun:test";
import { type Locks, leaderLive, type LeaderLive } from "../live/leader";
import { type Action, initialLive, reduce } from "../store";
import { Sources, settle, sig } from "./fakeSource";
import { OTHER, RUN } from "./fixtures";

class FakeLocks implements Locks {
	private busy = false;
	private queue: (() => Promise<void>)[] = [];
	request(_name: string, callback: () => Promise<void>): Promise<unknown> {
		this.queue.push(callback);
		this.next();
		return Promise.resolve();
	}
	private next() {
		if (this.busy) return;
		const run = this.queue.shift();
		if (run === undefined) return;
		this.busy = true;
		void run().then(() => {
			this.busy = false;
			this.next();
		});
	}
}

let channels = 0;

function tabs(n: number) {
	const name = `cockpit-test-${++channels}`;
	const sources = new Sources();
	const locks = new FakeLocks();
	const made = Array.from({ length: n }, (_, i) => {
		const actions: Action[] = [];
		const live: LeaderLive = leaderLive({ channel: new BroadcastChannel(name), locks, open: sources.open, tab: `tab-${i}`, every: () => () => {} });
		return { live, actions, push: (a: Action) => actions.push(a) };
	});
	return { sources, made };
}

const signals = (actions: Action[]) => reduce(initialLive(RUN), actions).signals.map((s) => s.seq);

describe("the leader fallback without a shared worker", () => {
	test("only the leader opens a connection per target and every tab receives the frames once", async () => {
		const { sources, made } = tabs(8);
		for (const t of made) t.live.subscribe(RUN, t.push);
		made[5]?.live.subscribe(OTHER, () => {});
		await settle(() => sources.live(RUN).length === 1 && sources.live(OTHER).length === 1, "one upstream per target");
		expect(made.map((t) => t.live.leading())).toEqual([true, false, false, false, false, false, false, false]);
		const s = sources.one(RUN);
		s.emit("signal", "e.0.1", sig(1));
		s.emit("signal", "e.0.1", sig(1));
		s.emit("signal", "e.0.2", sig(2));
		await settle(() => made.every((t) => signals(t.actions).length === 2), "frames everywhere");
		expect(made.map((t) => t.actions.filter((a) => a.kind === "frame").length)).toEqual([2, 2, 2, 2, 2, 2, 2, 2]);
		expect([sources.opened(RUN), sources.opened(OTHER)]).toEqual([1, 1]);
		for (const t of made) t.live.dispose();
	});

	test("when the leader closes the next tab leads, reopens the stream and the followers see no frame twice", async () => {
		const { sources, made } = tabs(3);
		for (const t of made) t.live.subscribe(RUN, t.push);
		made[2]?.live.subscribe(OTHER, () => {});
		await settle(() => sources.live(RUN).length === 1 && sources.live(OTHER).length === 1, "first upstreams");
		const first = sources.one(RUN);
		first.emit("signal", "e.0.1", sig(1));
		first.emit("signal", "e.0.2", sig(2));
		await settle(() => signals(made[2]?.actions ?? []).length === 2, "frames before the handover");
		made[0]?.live.dispose();
		await settle(() => made[1]?.live.leading() === true && sources.live(RUN).length === 1 && sources.opened(RUN) === 2, "second leader");
		await settle(() => sources.live(OTHER).length === 1 && sources.opened(OTHER) === 2, "a follower's own target reopened by the new leader");
		const second = sources.one(RUN);
		second.emit("signal", "e.0.1", sig(1));
		second.emit("signal", "e.0.2", sig(2));
		second.emit("signal", "e.0.3", sig(3));
		await settle(() => signals(made[2]?.actions ?? []).length === 3 && signals(made[1]?.actions ?? []).length === 3, "frames after the handover");
		const ids = (made[2]?.actions ?? []).flatMap((a) => (a.kind === "frame" ? [a.id] : []));
		expect([ids, first.closed]).toEqual([["e.0.1", "e.0.2", "e.0.3"], true]);
		for (const t of made.slice(1)) t.live.dispose();
	});

	test("a follower that drops its target releases it at the leader and a target nobody wants closes", async () => {
		const { sources, made } = tabs(2);
		const off = made[1]?.live.subscribe(OTHER, () => {});
		made[0]?.live.subscribe(RUN, () => {});
		await settle(() => sources.live(OTHER).length === 1 && sources.live(RUN).length === 1, "two upstreams");
		off?.();
		await settle(() => sources.live(OTHER).length === 0, "the dropped target closes");
		expect(sources.live(RUN).length).toBe(1);
		for (const t of made) t.live.dispose();
	});
});
