import { describe, expect, test } from "bun:test";
import { type ReadAfter, rereadOnResync } from "../api";
import type { ReadResponse, Signal } from "../shared/types";
import { type Action, type Frame, initialLive, reduce } from "../store";
import { OTHER, RUN } from "./fixtures";

const sig = (seq: number, collector = "toy-db"): Signal => ({ seq, ts: "2026-10-19T01:00:00Z", tsSource: "host", kind: "event", collector, attrs: {} });
const frame = (event: Frame["event"], id: string, data: unknown): Frame => ({ kind: "frame", event, id, data });
const page = (signals: Signal[], next: number, resync?: ReadResponse["resync"]): ReadResponse => ({ signals, frozen: false, frozenSeq: 0, dropped: {}, next, ...(resync ? { resync } : {}) });

function pagedRead(pages: Record<number, ReadResponse>) {
	const calls: [string, number][] = [];
	const read: ReadAfter = async (id, after) => {
		calls.push([id, after]);
		return pages[after] ?? page([], after);
	};
	return { read, calls };
}

async function drive(target: string | null, read: ReadAfter, frames: Action[]): Promise<Action[]> {
	const pushed: Action[] = [];
	const push = rereadOnResync(target, read, (a) => pushed.push(a));
	for (const f of frames) push(f);
	for (let i = 0; i < 50 && !pushed.some((a) => a.kind === "reread"); i++) await new Promise((r) => setTimeout(r, 1));
	return pushed;
}

describe("the screen re-reads the signals after a resync marker", () => {
	test("a resync of the selected run reads every page after the hole and the store shows the read signals", async () => {
		const { read, calls } = pagedRead({ 9: page([sig(10), sig(11)], 11), 11: page([sig(13)], 13, { from: 12, to: 12 }) });
		const frames: Action[] = [frame("signal", "e.0.1", sig(1)), frame("resync", "e.0.9", { target: RUN, from: 2, to: 9 })];
		const pushed = await drive(RUN, read, frames);
		expect(calls).toEqual([
			[RUN, 9],
			[RUN, 11],
			[RUN, 13],
		]);
		const s = reduce(initialLive(RUN), pushed);
		expect(s.signals.map((x) => x.seq)).toEqual([1, 10, 11, 13]);
		expect(s.holes).toEqual([
			{ from: 2, to: 9 },
			{ from: 12, to: 12 },
		]);
	});

	test("signals that arrived on the stream during the read are kept once, and a read of another run changes nothing", () => {
		let s = reduce(initialLive(RUN), [frame("signal", "e.0.1", sig(1)), frame("resync", "e.0.9", { target: RUN, from: 2, to: 9 }), frame("signal", "e.0.10", sig(10)), frame("signal", "e.0.12", sig(12))]);
		s = reduce(s, [{ kind: "reread", target: RUN, after: 9, signals: [sig(5), sig(10, "toy-cache"), sig(11)], holes: [] }]);
		expect(s.signals.map((x) => `${x.seq}:${x.collector}`)).toEqual(["1:toy-db", "10:toy-cache", "11:toy-db", "12:toy-db"]);
		const other = reduce(s, [{ kind: "reread", target: OTHER, after: 0, signals: [sig(99)], holes: [{ from: 50, to: 60 }] }]);
		expect(other).toBe(s);
	});

	test("a resync of another run, a plain signal frame or the hub target reads nothing", async () => {
		const { read, calls } = pagedRead({});
		const cases: [string | null, Action[]][] = [
			[RUN, [frame("resync", "e.0.9", { target: OTHER, from: 2, to: 9 })]],
			[RUN, [frame("signal", "e.0.1", sig(1))]],
			[null, [frame("resync", "e.0.9", { target: RUN, from: 2, to: 9 })]],
		];
		expect(cases.length).toBe(3);
		for (const [target, frames] of cases) {
			const pushed = await drive(target, read, frames);
			expect(pushed).toEqual(frames);
		}
		expect(calls).toEqual([]);
	});

	test("a failed read leaves the hole notice and the stream signals as they were", async () => {
		const read: ReadAfter = async () => {
			throw new Error("hub gone");
		};
		const frames: Action[] = [frame("signal", "e.0.1", sig(1)), frame("resync", "e.0.9", { target: RUN, from: 2, to: 9 })];
		const pushed = await drive(RUN, read, frames);
		expect(pushed).toEqual(frames);
		const s = reduce(initialLive(RUN), pushed);
		expect([s.signals.map((x) => x.seq), s.holes]).toEqual([[1], [{ from: 2, to: 9 }]]);
	});
});
