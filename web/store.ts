import type { HubList, JobEvent, JobFrame, RefusedFrame, Resync, ResyncFrame, RunsEvent, Signal } from "./shared/types";

export type JobLog = { jobId: string; title: string; lines: string[]; code?: number; running: boolean };
export type Connection = "connecting" | "open" | "lost";
export type Cursor = { epoch: string; bus: number; signal: number };

export const SIGNAL_CAP = 20_000;
export const STREAM_KINDS = ["runs", "job", "signal", "resync", "signal-refused"] as const;
export type StreamKind = (typeof STREAM_KINDS)[number];

export type Live = {
	target: string | null;
	hub: HubList | null;
	hubAt: string | null;
	connection: Connection;
	cursor: Cursor;
	jobs: JobLog[];
	signals: Signal[];
	holes: Resync[];
	refused: string | null;
};

export type Frame = { kind: "frame"; event: StreamKind; id: string; data: unknown };
export type Action = Frame | { kind: "select"; target: string | null } | { kind: "connection"; state: "open" | "lost" };

const NO_CURSOR: Cursor = { epoch: "", bus: 0, signal: 0 };
const NO_LINES = { jobs: [] as JobLog[], signals: [] as Signal[], holes: [] as Resync[], refused: null };

export function initialLive(target: string | null): Live {
	return { target, hub: null, hubAt: null, connection: "connecting", cursor: NO_CURSOR, ...NO_LINES };
}

export function parseCursor(id: string): Cursor | null {
	const parts = id.split(".");
	if (parts.length !== 3 || parts[0] === "") return null;
	const bus = Number(parts[1]);
	const signal = Number(parts[2]);
	if (!Number.isInteger(bus) || !Number.isInteger(signal) || bus < 0 || signal < 0) return null;
	return { epoch: parts[0] as string, bus, signal };
}

const cap = <T,>(list: T[], n: number) => (list.length > n ? list.slice(list.length - n) : list);

function applyJob(jobs: JobLog[], e: JobEvent): JobLog[] {
	if (e.phase === "start") return cap([...jobs, { jobId: e.jobId, title: e.title, lines: e.text ? [`$ ${e.text}`] : [], running: true }], 10);
	return jobs.map((j) => {
		if (j.jobId !== e.jobId) return j;
		if (e.phase === "exit") return { ...j, running: false, code: e.code };
		return { ...j, lines: cap([...j.lines, e.text ?? ""], 3000) };
	});
}

const lastSeq = (s: Live) => s.signals.at(-1)?.seq ?? 0;

function applyFrame(state: Live, f: Frame): Live {
	const cursor = parseCursor(f.id);
	if (cursor === null) return state;
	let s = state;
	if (s.cursor.epoch !== "" && cursor.epoch !== s.cursor.epoch) s = { ...s, jobs: [], cursor: { ...NO_CURSOR, epoch: cursor.epoch } };
	const busFrame = f.event === "runs" || f.event === "job";
	if (busFrame && s.cursor.epoch === cursor.epoch && cursor.bus <= s.cursor.bus) return s;
	s = { ...s, cursor: { epoch: cursor.epoch, bus: Math.max(s.cursor.bus, cursor.bus), signal: Math.max(s.cursor.signal, cursor.signal) } };
	switch (f.event) {
		case "runs": {
			const e = f.data as RunsEvent;
			return { ...s, hub: e.list, hubAt: e.at };
		}
		case "job": {
			const e = f.data as JobFrame;
			return e.target === s.target ? { ...s, jobs: applyJob(s.jobs, e) } : s;
		}
		case "signal": {
			const sig = f.data as Signal;
			return sig.seq > lastSeq(s) ? { ...s, signals: cap([...s.signals, sig], SIGNAL_CAP) } : s;
		}
		case "resync": {
			const hole = f.data as ResyncFrame;
			return s.holes.some((h) => h.from === hole.from && h.to === hole.to) ? s : { ...s, holes: [...s.holes, { from: hole.from, to: hole.to }] };
		}
		case "signal-refused":
			return { ...s, refused: (f.data as Partial<RefusedFrame>).reason ?? "signal reads are refused" };
	}
	return s;
}

function apply(s: Live, a: Action): Live {
	if (a.kind === "select") return a.target === s.target ? s : { ...initialLive(a.target), hub: s.hub, hubAt: s.hubAt };
	if (a.kind === "connection") return { ...s, connection: a.state === "open" ? "open" : "lost" };
	return applyFrame(s, a);
}

export function reduce(state: Live, actions: Action[]): Live {
	let next = state;
	for (const action of actions) next = apply(next, action);
	return next;
}
