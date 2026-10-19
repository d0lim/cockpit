import { describe, expect, test } from "bun:test";
import { Fanout } from "../live/fanout";
import { listen } from "../live/source";
import { localLive } from "../live/transport";
import type { Wire } from "../live/wire";
import { type Action, initialLive, reduce } from "../store";
import { FakeSource, Sources, sig } from "./fakeSource";
import { RUN } from "./fixtures";

const frames = (actions: Action[]) => actions.flatMap((a) => (a.kind === "frame" ? [a.id] : []));

describe("the source adapter", () => {
	test("it listens to every stream kind and forwards the raw frame with its id and target", () => {
		const source = new FakeSource("/api/events?target=hub");
		const got: Wire[] = [];
		listen(source, "hub", (w) => got.push(w));
		expect([...source.listeners.keys()].sort()).toEqual(["job", "resync", "runs", "signal", "signal-refused"]);
		source.onopen?.(new Event("open"));
		source.emit("signal", "e.0.1", sig(1));
		expect(got).toEqual([
			{ kind: "state", target: "hub", state: "open" },
			{ kind: "frame", target: "hub", event: "signal", id: "e.0.1", data: JSON.stringify(sig(1)) },
		]);
	});
});

describe("the fanout", () => {
	test("eight subscribers to one target share one upstream and a closing subscriber leaves the others fed", () => {
		const sources = new Sources();
		const got = new Map<number, string[]>();
		const fan = new Fanout<number>(sources.open, {
			live: (subs, w) => {
				for (const s of subs) got.get(s)?.push(w.kind === "frame" ? w.id : w.state);
			},
			replay: (s, ws) => got.set(s, ws.map((w) => (w.kind === "frame" ? w.id : w.state))),
		});
		for (let i = 0; i < 8; i++) fan.subscribe(i, RUN);
		fan.subscribe(0, "hub");
		expect([sources.opened(RUN), sources.opened("hub")]).toEqual([1, 1]);
		sources.one(RUN).emit("signal", "e.0.1", sig(1));
		fan.unsubscribe(3, RUN);
		sources.one(RUN).emit("signal", "e.0.2", sig(2));
		expect(got.get(3)).toEqual(["e.0.1"]);
		expect(got.get(7)).toEqual(["e.0.1", "e.0.2"]);
		for (const i of [0, 1, 2, 4, 5, 6]) fan.unsubscribe(i, RUN);
		expect(sources.live(RUN).length).toBe(1);
		fan.unsubscribe(7, RUN);
		expect([sources.live(RUN).length, sources.live("hub").length, sources.opened(RUN)]).toEqual([0, 1, 1]);
	});

	test("a late subscriber gets the state, the frames in arrival order and only the newest run list", () => {
		const sources = new Sources();
		const replays: string[][] = [];
		const fan = new Fanout<string>(sources.open, { live: () => {}, replay: (_s, ws) => replays.push(ws.map((w) => (w.kind === "frame" ? `${w.event}@${w.id}` : w.state))) });
		fan.subscribe("a", RUN);
		const s = sources.one(RUN);
		s.onopen?.(new Event("open"));
		s.emit("runs", "e.1.0", { list: 1 });
		s.emit("job", "e.2.0", { jobId: "j" });
		s.emit("signal", "e.2.1", sig(1));
		s.emit("runs", "e.3.1", { list: 2 });
		fan.subscribe("b", RUN);
		expect(replays).toEqual([[], ["open", "job@e.2.0", "signal@e.2.1", "runs@e.3.1"]]);
	});

	test("frames resent after a reconnect are not delivered twice and a new hub epoch starts over", () => {
		const sources = new Sources();
		const seen: string[] = [];
		const fan = new Fanout<string>(sources.open, { live: (_s, w) => w.kind === "frame" && seen.push(w.id), replay: () => {} });
		fan.subscribe("a", RUN);
		const s = sources.one(RUN);
		for (const [kind, id] of [["runs", "e.1.0"], ["signal", "e.1.1"], ["signal", "e.1.2"]] as const) s.emit(kind, id, kind === "signal" ? sig(1) : {});
		s.onerror?.(new Event("error"));
		for (const [kind, id] of [["runs", "e.1.0"], ["signal", "e.1.1"], ["signal", "e.1.2"], ["signal", "e.1.3"], ["job", "e.2.3"], ["signal-refused", "e.2.3"], ["signal-refused", "e.2.3"]] as const) s.emit(kind, id, {});
		s.emit("runs", "f.1.0", {});
		s.emit("signal", "f.1.1", sig(1));
		expect(seen).toEqual(["e.1.0", "e.1.1", "e.1.2", "e.1.3", "e.2.3", "e.2.3", "f.1.0", "f.1.1"]);
		expect(seen.length).toBe(8);
	});

	test("the replay log keeps only the newest frames past its cap", () => {
		const sources = new Sources();
		const replays: number[] = [];
		const fan = new Fanout<string>(sources.open, { live: () => {}, replay: (_s, ws) => replays.push(ws.length) }, 3);
		fan.subscribe("a", RUN);
		for (let i = 1; i <= 5; i++) sources.one(RUN).emit("signal", `e.0.${i}`, sig(i));
		fan.subscribe("b", RUN);
		expect(replays).toEqual([0, 3]);
	});
});

describe("the per-tab fallback", () => {
	test("two hooks in one tab share one source per target and the store sees every frame once", () => {
		const sources = new Sources();
		const live = localLive(sources.open);
		const a: Action[] = [];
		const b: Action[] = [];
		const offA = live.subscribe(RUN, (x) => a.push(x));
		const offB = live.subscribe(RUN, (x) => b.push(x));
		const s = sources.one(RUN);
		s.onopen?.(new Event("open"));
		s.emit("signal", "e.0.1", sig(1));
		s.emit("signal", "e.0.1", sig(1));
		s.emit("signal", "e.0.2", "{broken");
		s.emit("signal", "e.0.3", sig(3));
		offA();
		expect(s.closed).toBe(false);
		offB();
		expect([s.closed, sources.opened(RUN)]).toEqual([true, 1]);
		expect(frames(a)).toEqual(["e.0.1", "e.0.3"]);
		const state = reduce(initialLive(RUN), b);
		expect([state.signals.map((x) => x.seq), state.connection]).toEqual([[1, 3], "open"]);
	});
});
