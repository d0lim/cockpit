import { FLOW_AT, FLOW_COLUMNS, flowCells, rowClassOf, STATUS_AT, STATUS_TONES } from "../shared/flowTable";
import type { FlowSummary } from "../shared/types";
import { Badge } from "../ui";

type Props = { flows: FlowSummary[]; selected: string | null; onSelect: (flow: string | null) => void };

export function FlowTable({ flows, selected, onSelect }: Props) {
	if (flows.length === 0) return <p className="muted">No flow results</p>;
	return (
		<div className="table-wrap">
			<table>
				<thead>
					<tr>
						{FLOW_COLUMNS.map((c) => (
							<th key={c}>{c}</th>
						))}
					</tr>
				</thead>
				<tbody>
					{flows.map((f) => {
						const open = f.id === selected;
						const content = (cell: string, i: number) => {
							if (i === STATUS_AT) return <Badge tone={STATUS_TONES[f.status]}>{cell}</Badge>;
							if (i !== FLOW_AT) return cell;
							return (
								<button type="button" className="link" aria-pressed={open} title="Show steps and code" onClick={() => onSelect(open ? null : f.id)}>
									{cell}
								</button>
							);
						};
						return (
							<tr key={f.id} className={[rowClassOf(f.status), open ? "selected" : undefined].filter(Boolean).join(" ") || undefined}>
								{flowCells(f).map((cell, i) => (
									<td key={FLOW_COLUMNS[i]}>{content(cell, i)}</td>
								))}
							</tr>
						);
					})}
				</tbody>
			</table>
		</div>
	);
}
