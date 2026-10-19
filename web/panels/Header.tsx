import type { HubList, RunItem } from "../shared/types";
import type { Connection } from "../store";
import { Badge, time } from "../ui";

type Props = { run: string | null; item: RunItem | undefined; hub: HubList | null; hubAt: string | null; connection: Connection; onSelect: (id: string | null) => void };

export function Header({ run, item, hub, hubAt, connection, onSelect }: Props) {
	return (
		<header className="top">
			<div className="brand">
				<h1>cockpit</h1>
				<div className="picker">
					<button type="button" className={run === null ? "active" : undefined} onClick={() => onSelect(null)}>
						Runs
					</button>
					<select value={run ?? ""} onChange={(e) => onSelect(e.target.value || null)} aria-label="Pick a run">
						<option value="">Pick a run…</option>
						{run !== null && !item ? <option value={run}>{run}</option> : null}
						{hub?.runs.map((r) => (
							<option key={r.id} value={r.id}>
								{r.id} · {r.phaseLabel}
							</option>
						))}
					</select>
				</div>
				{run !== null ? <span className="muted">compose project {run}</span> : null}
				{run !== null && hub !== null && item === undefined ? <Badge tone="warn">not in the run list</Badge> : null}
			</div>
			<div className="actions">
				{connection === "lost" ? <Badge tone="bad">disconnected · last update {hubAt ? time(hubAt) : "none"}</Badge> : null}
				{connection === "connecting" ? <Badge tone="idle">connecting</Badge> : null}
			</div>
		</header>
	);
}
