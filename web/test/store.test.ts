import { describe, expect, test } from "bun:test";
import type { JobEvent, JobFrame, JobPhase, Signal } from "../shared/types";
import { type Action, type Frame, initialLive, parseCursor, reduce } from "../store";
import { hub, OTHER, RUN, run } from "./fixtures";

const frame = (event: Frame["event"], id: string, data: unknown): Frame => ({ kind: "frame", event, id, data });
const job = (id: string, phase: JobPhase, extra: Partial<JobEvent> = {}, target = RUN) => {
	const data: JobFrame = { kind: "job", target, at: "t", jobId: RUN, title: "pipeline", phase, ...extra };
	return frame("job", id, data);
};
const sig = (seq: number): Signal => ({ seq, ts: "2026-10-19T01:00:00Z", tsSource: "host", kind: "event", collector: "toy-db", attrs: {} });

describe("the live store folds stream frames", () => {
	test("job output folds start, lines and exit in order", () => {
		const s = reduce(initialLive(RUN), [job("e.1.0", "start", { text: "./run" }), job("e.2.0", "out", { text: "ok" }), job("e.3.0", "exit", { code: 0 })]);
		expect(s.jobs).toEqual([{ jobId: RUN, title: "pipeline", lines: ["$ ./run", "ok"], running: false, code: 0 }]);
	});

	test("job frames of another target are ignored while frames of the selected run are kept", () => {
		const s = reduce(initialLive(RUN), [job("e.1.0", "start", {}, OTHER), job("e.2.0", "start")]);
		expect(s.jobs.length).toBe(1);
	});

	test("the run list is replaced whole and kept when another run is selected", () => {
		let s = reduce(initialLive(RUN), [frame("runs", "e.1.0", { kind: "runs", target: "hub", at: "t1", list: hub([run({ position: 1, phase: "queued" })]) })]);
		s = reduce(s, [frame("runs", "e.2.0", { kind: "runs", target: "hub", at: "t2", list: hub([run()]) })]);
		expect(s.hub?.runs).toEqual([run()]);
		expect(s.hubAt).toBe("t2");
		s = reduce(s, [{ kind: "select", target: OTHER }]);
		expect(s.target).toBe(OTHER);
		expect(s.jobs).toEqual([]);
		expect(s.hub?.runs.map((r) => r.id)).toEqual([RUN]);
	});

	test("a reconnect that resends frames up to Last-Event-ID adds only the frames after it", () => {
		const first: Action[] = [{ kind: "connection", state: "open" }, job("e.1.0", "start"), frame("signal", "e.1.1", sig(1)), frame("signal", "e.1.2", sig(2)), job("e.2.2", "out", { text: "a" })];
		let s = reduce(initialLive(RUN), first);
		s = reduce(s, [{ kind: "connection", state: "lost" }]);
		expect(s.connection).toBe("lost");
		s = reduce(s, [{ kind: "connection", state: "open" }, job("e.2.2", "out", { text: "a" }), frame("signal", "e.2.2", sig(2)), frame("signal", "e.2.3", sig(3)), job("e.3.3", "out", { text: "b" })]);
		expect(s.signals.map((x) => x.seq)).toEqual([1, 2, 3]);
		expect(s.jobs[0]?.lines).toEqual(["a", "b"]);
		expect(s.cursor).toEqual({ epoch: "e", bus: 3, signal: 3 });
	});

	test("a new hub epoch replays from the start without doubling job lines or signals", () => {
		let s = reduce(initialLive(RUN), [job("old.1.0", "start"), job("old.2.0", "out", { text: "a" }), frame("signal", "old.2.1", sig(1))]);
		s = reduce(s, [job("new.1.0", "start"), job("new.2.0", "out", { text: "a" }), frame("signal", "new.2.1", sig(1))]);
		expect(s.jobs.length).toBe(1);
		expect(s.jobs[0]?.lines).toEqual(["a"]);
		expect(s.signals.length).toBe(1);
	});

	test("a resync marker is recorded once and a refusal keeps its reason", () => {
		const s = reduce(initialLive(RUN), [
			frame("resync", "e.0.9", { target: RUN, from: 3, to: 9 }),
			frame("resync", "e.0.9", { target: RUN, from: 3, to: 9 }),
			frame("signal-refused", "e.0.9", { target: RUN, reason: "signal reads are refused for this run" }),
		]);
		expect(s.holes).toEqual([{ from: 3, to: 9 }]);
		expect(s.refused).toBe("signal reads are refused for this run");
	});

	test("cursor ids are epoch, bus and signal numbers and anything else is rejected", () => {
		expect(parseCursor("abc.4.7")).toEqual({ epoch: "abc", bus: 4, signal: 7 });
		const bad = ["", "abc.4", ".1.2", "abc.x.1", "abc.-1.2"];
		expect(bad.map(parseCursor)).toEqual([null, null, null, null, null]);
	});
});
