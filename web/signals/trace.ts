import type { Signal, SpanStatus, TimeSource } from "../shared/types";

export type SpanSignal = Signal & { span: NonNullable<Signal["span"]> };

export type TraceSummary = {
	traceId: string;
	root: string;
	start: string;
	durationMs: number | null;
	status: SpanStatus;
	spans: number;
	exec: boolean;
};

export type SpanRow = {
	span: SpanSignal;
	spanId: string;
	depth: number;
	durationMs: number | null;
	offset: number;
	width: number;
	marker: boolean;
	incomplete: boolean;
	sourceMark: string | null;
};

const RANK: Readonly<Record<SpanStatus, number>> = { ok: 0, incomplete: 1, error: 2 };
const SOURCE_MARKS: Readonly<Record<TimeSource, string>> = { host: "H", container: "C", runner: "R" };
export const MIN_WIDTH = 0.5;

export const isSpan = (s: Signal): s is SpanSignal => s.kind === "span" && s.span !== undefined;
export const spanIdOf = (s: SpanSignal): string => s.id ?? `seq-${s.seq}`;

const ms = (iso: string | undefined): number | null => {
	if (!iso) return null;
	const t = Date.parse(iso);
	return Number.isNaN(t) ? null : t;
};

export function durationOf(span: SpanSignal["span"]): number | null {
	const a = ms(span.start);
	const b = ms(span.end);
	return a === null || b === null ? null : Math.max(0, b - a);
}

export function spansByTrace(signals: readonly Signal[]): Map<string, SpanSignal[]> {
	const out = new Map<string, SpanSignal[]>();
	for (const s of signals) if (isSpan(s)) out.set(s.span.traceId, [...(out.get(s.span.traceId) ?? []), s]);
	return out;
}

function roots(spans: readonly SpanSignal[]): SpanSignal[] {
	const ids = new Set(spans.map(spanIdOf));
	return spans.filter((s) => !s.span.parentId || !ids.has(s.span.parentId));
}

export function traceList(signals: readonly Signal[]): TraceSummary[] {
	const out: TraceSummary[] = [];
	for (const [traceId, spans] of spansByTrace(signals)) {
		const starts = spans.map((s) => ms(s.span.start)).filter((v): v is number => v !== null);
		const ends = spans.map((s) => ms(s.span.end)).filter((v): v is number => v !== null);
		const first = starts.length ? Math.min(...starts) : null;
		const last = ends.length ? Math.max(...ends) : null;
		const root = roots(spans)[0] ?? spans[0];
		out.push({
			traceId,
			root: root?.span.name ?? traceId,
			start: first === null ? "" : new Date(first).toISOString(),
			durationMs: first === null || last === null ? null : Math.max(0, last - first),
			status: spans.reduce<SpanStatus>((w, s) => (RANK[s.span.status] > RANK[w] ? s.span.status : w), "ok"),
			spans: spans.length,
			exec: traceId.includes("/exec/"),
		});
	}
	const recentFirst = (a: TraceSummary, b: TraceSummary) => b.start.localeCompare(a.start) || a.traceId.localeCompare(b.traceId);
	return [...out.filter((t) => t.exec).sort(recentFirst), ...out.filter((t) => !t.exec).sort(recentFirst)];
}

export function spanRows(spans: readonly SpanSignal[]): SpanRow[] {
	const byStart = (a: SpanSignal, b: SpanSignal) => (ms(a.span.start) ?? 0) - (ms(b.span.start) ?? 0) || a.seq - b.seq;
	const children = new Map<string, SpanSignal[]>();
	for (const s of spans) if (s.span.parentId) children.set(s.span.parentId, [...(children.get(s.span.parentId) ?? []), s]);
	const top = roots(spans).sort(byStart);
	const rootSource = top[0]?.tsSource;
	const starts = spans.map((s) => ms(s.span.start)).filter((v): v is number => v !== null);
	const ends = spans.map((s) => ms(s.span.end) ?? ms(s.span.start)).filter((v): v is number => v !== null);
	const t0 = starts.length ? Math.min(...starts) : 0;
	const total = Math.max(1, (ends.length ? Math.max(...ends) : t0) - t0);
	const rows: SpanRow[] = [];
	const seen = new Set<string>();
	const visit = (s: SpanSignal, depth: number) => {
		const id = spanIdOf(s);
		if (seen.has(id)) return;
		seen.add(id);
		const d = durationOf(s.span);
		const offset = (((ms(s.span.start) ?? t0) - t0) / total) * 100;
		const width = d === null ? 0 : (d / total) * 100;
		rows.push({
			span: s,
			spanId: id,
			depth,
			durationMs: d,
			offset,
			width: Math.max(width, MIN_WIDTH),
			marker: d === null || d === 0,
			incomplete: s.span.status === "incomplete",
			sourceMark: rootSource !== undefined && s.tsSource !== rootSource ? SOURCE_MARKS[s.tsSource] : null,
		});
		for (const c of (children.get(id) ?? []).sort(byStart)) visit(c, depth + 1);
	};
	for (const r of top) visit(r, 0);
	return rows;
}

export const durationText = (d: number | null): string => (d === null ? "no end" : d < 1000 ? `${d}ms` : `${(d / 1000).toFixed(1)}s`);

export const statusText = (row: Pick<SpanRow, "incomplete" | "span">): string => (row.incomplete ? "incomplete" : row.span.span.status);
