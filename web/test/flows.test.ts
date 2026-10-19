import { describe, expect, test } from "bun:test";
import { codeTarget, FLOW_COLUMNS, flowCells, flowRanges, highlightRange, jumpLine, NO_SOURCE, notProvenItems, rangeText, rowClassOf, stepFacts } from "../shared/flowTable";
import type { FlowSummary } from "../shared/types";

const FILE = "flows/transfer.ts";
const flow = (over: Partial<FlowSummary> = {}): FlowSummary => ({
	id: "transfer-allow",
	stage: "core",
	title: "Transfer is allowed",
	once: false,
	requires: [],
	source: { file: FILE, line: 12 },
	stepList: [],
	status: "passed",
	steps: 3,
	passed: 3,
	failed: 0,
	skipped: 0,
	...over,
});

describe("the flow view's code rules", () => {
	test("each flow in a file gets the range up to the next flow and the picked one is written out", () => {
		const flows = [flow(), flow({ id: "transfer-deny", source: { file: FILE, line: 41 } }), flow({ id: "elsewhere", source: { file: "flows/other.ts", line: 3 } })];
		const ranges = flowRanges(flows, FILE, 90);
		expect(ranges).toEqual([
			{ flow: "transfer-allow", start: 12, end: 40 },
			{ flow: "transfer-deny", start: 41, end: 90 },
		]);
		const picked = highlightRange(ranges, "transfer-deny");
		expect(picked).not.toBeNull();
		expect(picked && rangeText(picked)).toBe("flow range lines 41-90");
		expect(highlightRange(ranges, "elsewhere")).toBeNull();
	});

	test("picking only a flow opens its flow line without moving focus, and a step opens its own line with focus", () => {
		expect(codeTarget(flow(), null)).toEqual({ file: FILE, line: 12, focus: false });
		expect(codeTarget(flow(), { source: { file: "flows/lib/poll.ts", line: 9 } })).toEqual({ file: "flows/lib/poll.ts", line: 9, focus: true });
		expect(codeTarget(flow(), { source: undefined })).toEqual({ file: FILE, line: 12, focus: false });
	});

	test("a run without source locations has no file to open and says why", () => {
		expect(codeTarget(flow({ source: undefined }), null)).toEqual({ gap: NO_SOURCE });
	});

	test("a line past the end of the file or before line one is not jumped to", () => {
		const view = { file: FILE, lines: 90 };
		const jumps = [57, 90, 91, 0].map((line) => jumpLine({ file: FILE, line, focus: true }, view));
		expect(jumps).toEqual([57, 90, null, null]);
		expect(jumpLine({ file: "other.ts", line: 5, focus: true }, view)).toBeNull();
	});

	test("step facts always carry the request count and add duration and status only when present", () => {
		expect(stepFacts({ requests: 2 })).toBe("requests 2");
		expect(stepFacts({ requests: 1, durationMs: 40, httpStatus: 403 })).toBe("requests 1 · 40ms · HTTP 403");
	});

	test("flow rows have one cell per column and only passed rows are not painted as failed", () => {
		const cells = flowCells(flow({ requires: ["boot"], once: true, status: "failed", stoppedAt: "s2" }));
		expect(cells.length).toBe(FLOW_COLUMNS.length);
		expect(cells).toEqual(["core", "transfer-allow", "Failed", "3/3", "0", "0", "s2", "Transfer is allowed", "boot", "once"]);
		expect([rowClassOf("passed"), rowClassOf("skipped")]).toEqual([undefined, "failed"]);
	});

	test("not proven items list skipped steps, declared items and unread records", () => {
		expect(notProvenItems({ skipped: ["a/b"], declared: ["deposit"], unread: ["port:docker"] })).toEqual([
			"skipped step · a/b",
			"declared by the suite · deposit · Not proven",
			"unread record · port:docker",
		]);
		expect(notProvenItems(undefined)).toEqual([]);
	});
});
