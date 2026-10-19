import { type CSSProperties, useEffect, useRef } from "react";
import { fetchSource, usePoll } from "../api";
import { type CodeTarget, flowRanges, highlightRange, jumpLine, rangeText } from "../shared/flowTable";
import type { FlowSummary, SourceView } from "../shared/types";
import { Badge, Panel } from "../ui";
import { SOURCE_BADGES } from "./sourceBadge";

export const CodeGap = ({ sentence }: { sentence: string }) => <p className="needs">no code · {sentence}</p>;

type Props = { id: string; flow: string; flows: FlowSummary[]; target: CodeTarget; version: string };

export function CodeView({ id, flow, flows, target, version }: Props) {
	const source = usePoll(() => fetchSource(id, target.file), null, [id, target.file, version]);
	const view = source.data;
	const badge = view ? SOURCE_BADGES[view.source] : null;
	return (
		<Panel title={`Code · ${target.file}`} actions={badge ? <Badge tone={badge.tone}>{badge.label}</Badge> : undefined}>
			{view === null ? source.error ? <CodeGap sentence={source.error} /> : <p className="muted">Loading</p> : <CodeBody view={view} flow={flow} flows={flows} target={target} />}
		</Panel>
	);
}

export function CodeBody({ view, flow, flows, target }: { view: SourceView; flow: string; flows: FlowSummary[]; target: CodeTarget }) {
	const box = useRef<HTMLDivElement>(null);
	const line = jumpLine(target, view);
	const range = highlightRange(flowRanges(flows, view.file, view.lines), flow);
	const { focus } = target;
	useEffect(() => {
		const el = box.current;
		const row = line === null ? null : el?.querySelector<HTMLElement>(`[data-line="${line}"]`);
		if (!el || !row) return;
		el.scrollTop = Math.max(0, row.offsetTop - el.clientHeight / 3);
		if (focus) row.focus();
	}, [line, focus, view.file, view.text]);

	const rows = view.text.split("\n").slice(0, view.lines);
	const width = { "--ln": `${String(view.lines).length}ch` } as CSSProperties;
	return (
		<>
			<p className="needs">
				{range ? rangeText(range) : "this file does not define the picked flow"}
				{line === null ? null : ` · now at line ${line}`}
			</p>
			{focus && line === null ? (
				<p className="needs">
					The picked step's line ({target.line}) is past the end of this file ({view.lines} lines)
				</p>
			) : null}
			<div className="code" ref={box} style={width} role="region" aria-label={`${view.file} source`} tabIndex={0}>
				<div className="code-lines">
					{rows.map((text, i) => {
						const n = i + 1;
						const here = n === line;
						const inFlow = range !== null && n >= range.start && n <= range.end;
						return (
							<div
								key={n}
								data-line={n}
								className={`code-line${inFlow ? " in-flow" : ""}${here ? " here" : ""}`}
								tabIndex={here ? -1 : undefined}
								aria-current={here ? "location" : undefined}
							>
								<span className="bar" aria-hidden="true" />
								<span className="ln">{n}</span>
								<span className="src">{text}</span>
							</div>
						);
					})}
				</div>
			</div>
		</>
	);
}
