import type { HubList, RunItem, Signal } from "../shared/types";

export const RUN = "run-20261019-010203-abcdef1";
export const OTHER = "run-20261019-020304-1234567";

export const run = (over: Partial<RunItem> = {}): RunItem => ({
	id: RUN,
	phase: "testing",
	env: "up",
	verdict: null,
	sha: "a".repeat(40),
	suite: "toy",
	hostApi: 1,
	cockpitVersion: "0.1.0",
	submitter: "ui",
	keep: false,
	createdAt: "2026-10-19T01:02:03.000Z",
	slot: 0,
	phaseLabel: "Running",
	verdictLabel: null,
	...over,
});

export const hub = (runs: RunItem[], over: Partial<HubList> = {}): HubList => ({
	runs,
	orphans: [],
	pool: { size: 1, max: 5 },
	up: { count: 1, cap: 3 },
	dockerDown: false,
	tickError: null,
	...over,
});

let seq = 0;
export const resetSeq = () => {
	seq = 0;
};

export const event = (attrs: Signal["attrs"], over: Partial<Signal> = {}): Signal => {
	seq++;
	return { seq, ts: new Date(Date.UTC(2026, 9, 19, 1, 0, seq)).toISOString(), tsSource: "host", kind: "event", collector: "toy-db", attrs, event: { body: `body ${seq}` }, ...over };
};

export const span = (id: string, parentId: string | undefined, start: number, end: number | undefined, over: Partial<Signal> = {}, status: "ok" | "error" | "incomplete" = "ok"): Signal => {
	seq++;
	const at = (s: number) => new Date(Date.UTC(2026, 9, 19, 1, 0, 0) + s).toISOString();
	return {
		seq,
		ts: at(start),
		tsSource: "runner",
		kind: "span",
		collector: "runner",
		id,
		attrs: {},
		span: { traceId: RUN, parentId, name: id, start: at(start), end: end === undefined ? undefined : at(end), status },
		...over,
	};
};
