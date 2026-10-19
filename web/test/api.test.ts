import { describe, expect, test } from "bun:test";
import { attach, type Source } from "../api";
import { type Action, initialLive, reduce } from "../store";
import { RUN } from "./fixtures";

class FakeSource implements Source {
	onopen: ((ev: Event) => unknown) | null = null;
	onerror: ((ev: Event) => unknown) | null = null;
	listeners = new Map<string, (msg: MessageEvent) => void>();
	closed = false;
	addEventListener(kind: string, fn: (msg: MessageEvent) => void) {
		this.listeners.set(kind, fn);
	}
	close() {
		this.closed = true;
	}
	emit(kind: string, id: string, data: unknown) {
		this.listeners.get(kind)?.(new MessageEvent(kind, { data: JSON.stringify(data), lastEventId: id }));
	}
}

const sig = (seq: number) => ({ seq, ts: "2026-10-19T01:00:00Z", tsSource: "host", kind: "event", collector: "toy-db", attrs: {} });

describe("the event stream adapter", () => {
	test("it listens to every stream kind, parses frames and survives a malformed one", () => {
		const source = new FakeSource();
		const got: Action[] = [];
		attach(source, (a) => got.push(a));
		expect([...source.listeners.keys()].sort()).toEqual(["job", "resync", "runs", "signal", "signal-refused"]);
		source.emit("signal", "e.0.1", sig(1));
		source.listeners.get("signal")?.(new MessageEvent("signal", { data: "{not json", lastEventId: "e.0.2" }));
		expect(got).toEqual([{ kind: "frame", event: "signal", id: "e.0.1", data: sig(1) }]);
	});

	test("after the browser reconnects with Last-Event-ID only the frames after it are added and none repeat", () => {
		const first = new FakeSource();
		const actions: Action[] = [];
		attach(first, (a) => actions.push(a));
		first.onopen?.(new Event("open"));
		first.emit("signal", "e.0.1", sig(1));
		first.emit("signal", "e.0.2", sig(2));
		first.onerror?.(new Event("error"));
		first.onopen?.(new Event("open"));
		first.emit("signal", "e.0.2", sig(2));
		first.emit("signal", "e.0.3", sig(3));
		const s = reduce(initialLive(RUN), actions);
		expect(s.signals.map((x) => x.seq)).toEqual([1, 2, 3]);
		expect(s.connection).toBe("open");
	});
});
