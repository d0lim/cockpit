import { type KeyboardEvent, useState } from "react";
import type { TracePick } from "../nav";
import type { Signal } from "../shared/types";
import { PLACEMENT_LABELS, placement } from "../signals/model";
import { durationText, type SpanRow, spanRows, spansByTrace, statusText, traceList } from "../signals/trace";
import type { SignalMeta } from "./SignalFrame";
import { TraceList } from "./TraceList";

type Props = { rows: readonly Signal[]; meta: SignalMeta; pick: TracePick; onPick: (pick: TracePick) => void };

export function SignalTrace({ rows, meta, pick, onPick }: Props) {
	const [back, setBack] = useState<string[]>([]);
	const traces = traceList(rows);
	const trace = pick.trace !== null && traces.some((t) => t.traceId === pick.trace) ? pick.trace : (traces[0]?.traceId ?? null);
	const spans = trace === null ? [] : (spansByTrace(rows).get(trace) ?? []);
	const lines = spanRows(spans);
	const picked = lines.find((l) => l.spanId === pick.span) ?? null;
	const follow = (to: string) => {
		if (trace !== null) setBack((cur) => [...cur, trace]);
		onPick({ trace: to, span: null });
	};
	const goBack = () => {
		const prev = back.at(-1);
		if (prev === undefined) return;
		setBack((cur) => cur.slice(0, -1));
		onPick({ trace: prev, span: null });
	};
	const move = (e: KeyboardEvent, i: number) => {
		const next = e.key === "ArrowDown" ? lines[i + 1] : e.key === "ArrowUp" ? lines[i - 1] : undefined;
		if (next === undefined) return;
		e.preventDefault();
		onPick({ trace, span: next.spanId });
	};
	return (
		<div className="signal-view">
			<TraceList traces={traces} picked={trace} onPick={(t) => onPick({ trace: t, span: null })} />
			{back.length > 0 ? (
				<button type="button" className="small" onClick={goBack}>
					Back to {back.at(-1)}
				</button>
			) : null}
			<ol className="spans" aria-label={`spans of ${trace ?? "no trace"}`}>
				{lines.map((l, i) => (
					<li key={l.spanId}>
						<SpanLine line={l} meta={meta} selected={l.spanId === pick.span} onSelect={() => onPick({ trace, span: l.spanId })} onKey={(e) => move(e, i)} />
					</li>
				))}
			</ol>
			{picked ? <SpanDetail line={picked} onFollow={follow} /> : null}
		</div>
	);
}

type LineProps = { line: SpanRow; meta: SignalMeta; selected: boolean; onSelect: () => void; onKey: (e: KeyboardEvent) => void };

export function SpanLine({ line, meta, selected, onSelect, onKey }: LineProps) {
	const where = PLACEMENT_LABELS[placement(line.span.seq, meta.frozen, meta.frozenSeq)];
	return (
		<button type="button" className={selected ? "span-row selected" : "span-row"} aria-pressed={selected} onClick={onSelect} onKeyDown={onKey}>
			<span className="span-name" style={{ paddingLeft: `${line.depth * 1.2}em` }}>
				{line.span.span.name}
			</span>
			<span className="span-track" aria-hidden="true">
				<span className={line.marker ? "span-bar marker" : "span-bar"} style={{ marginLeft: `${line.offset}%`, width: `${line.width}%` }} />
			</span>
			<span className="span-facts">
				{durationText(line.durationMs)} · {statusText(line)}
				{line.sourceMark ? ` · time ${line.sourceMark}` : ""}
				{where ? ` · ${where}` : ""}
			</span>
		</button>
	);
}

function SpanDetail({ line, onFollow }: { line: SpanRow; onFollow: (trace: string) => void }) {
	const { span } = line.span;
	return (
		<aside className="detail" aria-label="Span detail">
			<dl className="kv">
				<dt>span</dt>
				<dd>{line.spanId}</dd>
				<dt>start</dt>
				<dd>{span.start}</dd>
				<dt>end</dt>
				<dd>{span.end ?? "not ended"}</dd>
				<dt>status</dt>
				<dd>{statusText(line)}</dd>
				<dt>time source</dt>
				<dd>{line.span.tsSource}</dd>
			</dl>
			{(span.links ?? []).map((link) => (
				<button key={`${link.traceId}/${link.spanId}`} type="button" className="small" onClick={() => onFollow(link.traceId)}>
					Follow the link to {link.traceId}
				</button>
			))}
		</aside>
	);
}
