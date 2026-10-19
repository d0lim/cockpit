import { useEffect, useReducer, useState } from "react";
import type { FlowView, HubList, KitView, OpReply, Problem, ReadResponse, ReportView, Resync, ResyncFrame, RunDetail, RunOp, Signal, SignalsView, SourceView } from "./shared/types";
import { chooseLive, type Env } from "./live/transport";
import type { Live } from "./live/router";
import { HUB_TARGET } from "./live/wire";
import { type Action, initialLive, type Live as LiveState, reduce, SIGNAL_CAP } from "./store";

export class ApiFailure extends Error {
	constructor(
		readonly status: number,
		readonly code: string,
		message: string,
		readonly body: Partial<Problem>,
	) {
		super(message);
		this.name = "ApiFailure";
	}
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, init);
	const body: unknown = await res.json().catch(() => null);
	if (res.ok) return body as T;
	const fields = typeof body === "object" && body !== null && !Array.isArray(body) ? (body as Record<string, unknown>) : {};
	const message = typeof fields.message === "string" ? fields.message : `HTTP ${res.status}`;
	throw new ApiFailure(res.status, typeof fields.code === "string" ? fields.code : `HTTP_${res.status}`, message, fields as Partial<Problem>);
}

export const post = <T,>(path: string, body?: unknown) =>
	call<T>(path, {
		method: "POST",
		headers: body === undefined ? { "x-cockpit": "1" } : { "x-cockpit": "1", "content-type": "application/json" },
		body: body === undefined ? undefined : JSON.stringify(body),
	});

const runPath = (id: string, rest = "") => `/api/runs/${encodeURIComponent(id)}${rest}`;

export const fetchRuns = () => call<HubList>("/api/runs");
export const fetchRun = (id: string) => call<RunDetail>(runPath(id));
export const fetchKit = (id: string) => call<KitView>(runPath(id, "/kit"));
export const fetchReport = (id: string) => call<ReportView>(runPath(id, "/report"));
export const fetchFlow = (id: string, flow: string) => call<FlowView>(runPath(id, `/flows/${encodeURIComponent(flow)}`));
export const fetchSource = (id: string, file: string) => call<SourceView>(runPath(id, `/source?file=${encodeURIComponent(file)}`));
export const fetchSignalMeta = (id: string, after: number) => call<SignalsView>(runPath(id, `/signals?after=${after}&limit=1`));
export const fetchSignalsAfter = (id: string, after: number) => call<SignalsView>(runPath(id, `/signals?after=${after}`));

export type ReadAfter = (id: string, after: number) => Promise<ReadResponse>;

export async function readAfterHole(target: string, to: number, read: ReadAfter): Promise<Action> {
	const signals: Signal[] = [];
	const holes: Resync[] = [];
	let after = to;
	while (signals.length < SIGNAL_CAP) {
		const r = await read(target, after);
		if (r.resync) holes.push(r.resync);
		signals.push(...r.signals);
		if (r.signals.length === 0 || r.next <= after) break;
		after = r.next;
	}
	return { kind: "reread", target, after: to, signals, holes };
}

export function rereadOnResync(target: string | null, read: ReadAfter, push: (a: Action) => void): (a: Action) => void {
	return (a) => {
		push(a);
		if (target === null || a.kind !== "frame" || a.event !== "resync") return;
		const hole = a.data as ResyncFrame;
		if (hole.target !== target) return;
		readAfterHole(target, hole.to, read).then(push, () => undefined);
	};
}

export const operate = (id: string, op: RunOp, force: boolean) => post<OpReply>(runPath(id, `/${op}${force ? "?force=1" : ""}`));

const FLUSH_MS = 50;

const browserEnv = (): Env => ({
	SharedWorker: typeof SharedWorker === "undefined" ? undefined : SharedWorker,
	BroadcastChannel: typeof BroadcastChannel === "undefined" ? undefined : BroadcastChannel,
	locks: typeof navigator === "undefined" ? undefined : navigator.locks,
	source: (url) => new EventSource(url),
	onHide: (fn) => addEventListener("pagehide", fn),
});

let shared: Live | null = null;

export const defaultLive = (): Live => {
	shared ??= chooseLive(browserEnv()).live;
	return shared;
};

export function useLive(target: string | null, source: Live = defaultLive(), read: ReadAfter = fetchSignalsAfter): LiveState {
	const [live, dispatch] = useReducer(reduce, target, initialLive);
	useEffect(() => {
		let queue: Action[] = [];
		let timer: ReturnType<typeof setTimeout> | undefined;
		const flush = () => {
			timer = undefined;
			const batch = queue;
			queue = [];
			dispatch(batch);
		};
		const push = (action: Action) => {
			queue.push(action);
			if (timer === undefined) timer = setTimeout(flush, FLUSH_MS);
		};
		dispatch([{ kind: "select", target }]);
		const off = source.subscribe(target ?? HUB_TARGET, rereadOnResync(target, read, push));
		return () => {
			off();
			clearTimeout(timer);
		};
	}, [target, source, read]);
	return live;
}

export function usePoll<T>(load: () => Promise<T>, ms: number | null, deps: unknown[]): { data: T | null; error: string | null } {
	const [data, setData] = useState<T | null>(null);
	const [error, setError] = useState<string | null>(null);
	useEffect(() => {
		let alive = true;
		let loading = false;
		let last: string | null = null;
		const run = () => {
			if (loading) return;
			loading = true;
			load()
				.then((d) => {
					if (!alive) return;
					const text = JSON.stringify(d);
					if (text !== last) {
						last = text;
						setData(d);
					}
					setError(null);
				})
				.catch((e: Error) => alive && setError(e.message))
				.finally(() => {
					loading = false;
				});
		};
		run();
		const timer = ms === null ? undefined : setInterval(run, ms);
		return () => {
			alive = false;
			clearInterval(timer);
		};
	}, [...deps, ms]);
	return { data, error };
}
