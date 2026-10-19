import { beforeEach, describe, expect, test } from "bun:test";
import { NEUTRAL } from "../shared/classify";
import type { CollectorState } from "../shared/types";
import { chipsOf, eventRows, groups, matches, recent } from "../signals/events";
import { droppedLine, EMPTY_SENTENCES, emptyKind, latestRevisions, placement, withBoundaries } from "../signals/model";
import { tables } from "../signals/table";
import { spanRows, spansByTrace, traceList } from "../signals/trace";
import { event, RUN, resetSeq, run, span } from "./fixtures";

beforeEach(resetSeq);

describe("signal views read only the reserved attributes", () => {
	test("100 events group by status with the right counts and a status without tone is neutral", () => {
		const rows = Array.from({ length: 100 }, (_, i) =>
			i % 4 === 0 ? event({ status: "sent", tone: "ok" }) : i % 4 === 1 ? event({ status: "failed", tone: "bad" }) : i % 4 === 2 ? event({ status: "queued" }) : event({}),
		);
		const g = groups(rows);
		expect(g.map((x) => [x.status, x.count, x.tone])).toEqual([
			["sent", 25, "ok"],
			["failed", 25, "bad"],
			["queued", 25, NEUTRAL],
			["(no status)", 25, NEUTRAL],
		]);
		expect(g.reduce((n, x) => n + x.count, 0)).toBe(100);
	});

	test("chips filter by key and value and the window keeps only the latest events", () => {
		const rows = [event({ status: "sent", label: "a" }), event({ status: "failed", label: "b" }), event({ status: "sent", label: "c" })];
		expect(rows.filter((s) => matches(s, [{ key: "status", value: "sent" }])).map((s) => s.attrs.label)).toEqual(["a", "c"]);
		expect(chipsOf(rows).some((c) => c.key === "collector" && c.value === "toy-db")).toBe(true);
		expect(recent(rows, 2).map((s) => s.attrs.label)).toEqual(["b", "c"]);
	});

	test("only the highest revision of an id is shown and a tie goes to the later seq", () => {
		const v1 = event({ status: "queued" }, { id: "n-1", rev: 1 });
		const v3 = event({ status: "sent" }, { id: "n-1", rev: 3 });
		const v2 = event({ status: "late" }, { id: "n-1", rev: 2 });
		const t1 = event({ status: "a" }, { id: "n-2", rev: 1 });
		const t2 = event({ status: "b" }, { id: "n-2", rev: 1 });
		const plain = event({ status: "x" });
		expect(latestRevisions([v1, v3, v2, t1, t2, plain]).map((s) => s.attrs.status)).toEqual(["sent", "b", "x"]);
	});

	test("tables split by the table attribute and their columns are the union of the row attributes", () => {
		const rows = [event({ table: "jobs", label: "a", status: "ok" }), event({ table: "jobs", label: "b", owner: "x" }), event({ table: "queues", label: "q", depth: 3 }), event({ status: "free" })];
		const t = tables(rows);
		expect(t.map((x) => [x.name, x.rows.length, x.columns])).toEqual([
			["jobs", 2, ["label", "status", "owner"]],
			["queues", 1, ["label", "depth"]],
		]);
	});

	test("container log lines are not events of the events view", () => {
		const rows = [event({ container: "app" }, { collector: "logs" }), event({ status: "sent" })];
		expect(eventRows(rows).length).toBe(1);
	});

	test("the picked trace keeps parents before children in start order and an incomplete span says so", () => {
		const rows = [span("step:b", "flow:f", 60, 90), span("run", undefined, 0, undefined, {}, "incomplete"), span("flow:f", "run", 10, 100), span("step:a", "flow:f", 20, 50), span("step:z", "flow:f", 95, 95)];
		const lines = spanRows(spansByTrace(rows).get(RUN) ?? []);
		expect(lines.map((l) => [l.spanId, l.depth])).toEqual([
			["run", 0],
			["flow:f", 1],
			["step:a", 2],
			["step:b", 2],
			["step:z", 2],
		]);
		expect(lines[0]?.incomplete).toBe(true);
		expect(lines.filter((l) => l.marker).map((l) => l.spanId)).toEqual(["run", "step:z"]);
	});

	test("the trace list puts the latest exec trace first and reports the worst status", () => {
		const rows = [
			span("run", undefined, 0, 100, {}, "error"),
			span("exec", undefined, 500, 600, { span: { traceId: `${RUN}/exec/one`, name: "exec", start: "2026-10-19T02:00:00Z", end: "2026-10-19T02:00:01Z", status: "ok" } }),
			span("exec", undefined, 700, 800, { span: { traceId: `${RUN}/exec/two`, name: "exec", start: "2026-10-19T03:00:00Z", end: "2026-10-19T03:00:02Z", status: "ok" } }),
		];
		const list = traceList(rows);
		expect(list.map((t) => t.traceId)).toEqual([`${RUN}/exec/two`, `${RUN}/exec/one`, RUN]);
		expect(list[2]?.status).toBe("error");
		expect(list[0]?.durationMs).toBe(2000);
	});

	test("a span whose time source differs from the root carries a source mark", () => {
		const rows = [span("run", undefined, 0, 100), span("probe", "run", 10, 20, { tsSource: "container" })];
		const lines = spanRows(spansByTrace(rows).get(RUN) ?? []);
		expect(lines.map((l) => l.sourceMark)).toEqual([null, "C"]);
	});
});

describe("boundaries and states", () => {
	test("the frozen seq splits rows with a divider and the dropped line heads the tail", () => {
		const rows = [event({}), event({}), event({})];
		const items = withBoundaries(rows, { frozen: true, frozenSeq: 2, dropped: { "tail-cap": 4 } });
		expect(items.map((i) => (i.kind === "row" ? i.row.seq : i.kind))).toEqual([1, 2, "divider", "dropped", 3]);
		expect(withBoundaries(rows, { frozen: false, frozenSeq: 0, dropped: {} }).map((i) => i.kind)).toEqual(["row", "row", "row"]);
		expect(droppedLine({ "tail-cap": 4, "container-quota": 1 })).toBe("5 older records dropped (tail-cap 4, container-quota 1)");
		expect(droppedLine({ "tail-cap": 0 })).toBeNull();
		expect([placement(2, true, 2), placement(3, true, 2), placement(3, false, 0)]).toEqual(["in-bundle", "after-bundle", "live"]);
	});

	test("the six empty states are chosen from the phase, the environment, the collectors and the other signals and read differently", () => {
		const armed: CollectorState[] = [{ id: "toy-db", state: "armed", failures: 0 }];
		const kinds = [
			emptyKind(run({ phase: "building", env: "absent" }), armed, 0),
			emptyKind(run({ phase: "done", env: "stopped" }), armed, 0),
			emptyKind(run({ phase: "done", env: "up" }), [], 0),
			emptyKind(run({ phase: "done", env: "up" }), armed, 0, 20),
			emptyKind(run({ phase: "done", env: "up" }), [{ ...armed[0]!, state: "stopped" }], 0),
			emptyKind(run({ phase: "done", env: "up" }), armed, 0),
		];
		expect(kinds).toEqual(["before-boot", "env-off", "no-collectors", "none-of-kind", "collectors-stopped", "no-signals"]);
		expect(new Set(kinds.map((k) => (k ? EMPTY_SENTENCES[k] : ""))).size).toBe(6);
		expect(emptyKind(run({ phase: "done", env: "up" }), [...armed, { id: "toy-queue", state: "stopped", failures: 0 }], 0)).toBe("no-signals");
		expect(emptyKind(run({ phase: "done", env: "up" }), armed, 3)).toBeNull();
		expect(emptyKind(run({ phase: "done", env: "stopped" }), armed, 3)).toBe("env-off");
	});
});
