import { useEffect, useReducer, useState } from "react";
import type { FlowView, HubList, KitView, OpReply, Problem, ReportView, RunDetail, RunOp, SignalsView, SourceView } from "./shared/types";
import { type Action, initialLive, type Live, reduce, STREAM_KINDS, type StreamKind } from "./store";

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
export const operate = (id: string, op: RunOp, force: boolean) => post<OpReply>(runPath(id, `/${op}${force ? "?force=1" : ""}`));

export const HUB_TARGET = "hub";
export const eventsUrl = (target: string | null) => `/api/events?target=${encodeURIComponent(target ?? HUB_TARGET)}`;

const FLUSH_MS = 50;

export type Source = {
	addEventListener(kind: string, fn: (msg: MessageEvent) => void): void;
	onopen: ((ev: Event) => unknown) | null;
	onerror: ((ev: Event) => unknown) | null;
	close(): void;
};

export function attach(source: Source, push: (a: Action) => void): void {
	source.onopen = () => push({ kind: "connection", state: "open" });
	source.onerror = () => push({ kind: "connection", state: "lost" });
	for (const event of STREAM_KINDS) {
		source.addEventListener(event, (msg) => {
			try {
				push({ kind: "frame", event: event as StreamKind, id: msg.lastEventId, data: JSON.parse(String(msg.data)) });
			} catch {
				return;
			}
		});
	}
}

const openEventSource = (url: string): Source => new EventSource(url);

export function useLive(target: string | null, open: (url: string) => Source = openEventSource): Live {
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
		const source = open(eventsUrl(target));
		attach(source, push);
		return () => {
			source.close();
			clearTimeout(timer);
		};
	}, [target, open]);
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
