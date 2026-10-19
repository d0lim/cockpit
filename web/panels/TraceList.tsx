import type { TraceSummary } from "../signals/trace";
import { durationText } from "../signals/trace";
import { Badge, stamp } from "../ui";

const STATUS_TONE = { ok: "ok", error: "bad", incomplete: "warn" } as const;

export function TraceList({ traces, picked, onPick }: { traces: readonly TraceSummary[]; picked: string | null; onPick: (trace: string) => void }) {
	if (traces.length === 0) return <p className="muted">No traces yet.</p>;
	return (
		<table className="trace-list">
			<thead>
				<tr>
					<th>root</th>
					<th>start</th>
					<th>length</th>
					<th>status</th>
					<th>spans</th>
				</tr>
			</thead>
			<tbody>
				{traces.map((t) => (
					<tr key={t.traceId} className={t.traceId === picked ? "selected" : undefined}>
						<td>
							<button type="button" className="link" aria-pressed={t.traceId === picked} onClick={() => onPick(t.traceId)}>
								{t.root}
							</button>
							{t.exec ? <span className="muted"> exec</span> : null}
						</td>
						<td>{stamp(t.start)}</td>
						<td>{durationText(t.durationMs)}</td>
						<td>
							<Badge tone={STATUS_TONE[t.status]}>{t.status}</Badge>
						</td>
						<td>{t.spans}</td>
					</tr>
				))}
			</tbody>
		</table>
	);
}
