import { useState } from "react";
import type { Signal } from "../shared/types";
import { PLACEMENT_LABELS, placement, withBoundaries } from "../signals/model";
import { cellOf, type SignalTableData, tables } from "../signals/table";
import { time } from "../ui";
import { DividerRow, DroppedRow } from "./Boundary";
import type { SignalMeta } from "./SignalFrame";

export function SignalTables({ rows, meta }: { rows: readonly Signal[]; meta: SignalMeta }) {
	const list = tables(rows);
	if (list.length === 0) return <p className="muted">No signal carries a table attribute yet.</p>;
	return (
		<>
			{list.map((t) => (
				<SignalTable key={t.name} data={t} meta={meta} />
			))}
		</>
	);
}

export function SignalTable({ data, meta, initialHidden = [] }: { data: SignalTableData; meta: SignalMeta; initialHidden?: string[] }) {
	const [hidden, setHidden] = useState<string[]>(initialHidden);
	const columns = data.columns.filter((c) => !hidden.includes(c));
	const span = columns.length + 2;
	const toggle = (c: string) => setHidden((cur) => (cur.includes(c) ? cur.filter((x) => x !== c) : [...cur, c]));
	return (
		<section className="signal-table" aria-label={`table ${data.name}`}>
			<h3>
				{data.name} <span className="muted">{data.rows.length} rows</span>
			</h3>
			<div className="chips" aria-label="Columns">
				{data.columns.map((c) => (
					<button key={c} type="button" className={hidden.includes(c) ? "chip" : "chip on"} aria-pressed={!hidden.includes(c)} onClick={() => toggle(c)}>
						{c}
					</button>
				))}
			</div>
			<div className="table-wrap">
				<table>
					<thead>
						<tr>
							<th>time</th>
							{columns.map((c) => (
								<th key={c}>{c}</th>
							))}
							<th>bundle</th>
						</tr>
					</thead>
					<tbody>
						{withBoundaries(data.rows, meta).map((item, i) => {
							if (item.kind === "divider") return <DividerRow key={`divider-${i}`} span={span} />;
							if (item.kind === "dropped") return <DroppedRow key={`dropped-${i}`} span={span} text={item.text} />;
							const s = item.row;
							return (
								<tr key={s.seq}>
									<td>{time(s.ts)}</td>
									{columns.map((c) => (
										<td key={c}>{cellOf(s, c)}</td>
									))}
									<td className="muted">{PLACEMENT_LABELS[placement(s.seq, meta.frozen, meta.frozenSeq)]}</td>
								</tr>
							);
						})}
					</tbody>
				</table>
			</div>
		</section>
	);
}
