import { label } from "../shared/labels";
import { nextDestroy, nextStop } from "../shared/pool";
import type { HubList, RunItem } from "../shared/types";
import { groupRuns, phaseTone, rowDetail, SUBMIT_HINT, verdictTone } from "../runs";
import { Badge, Panel } from "../ui";

export const poolUsage = (hub: HubList | null): string =>
	hub ? `kept ${hub.pool.size}/${hub.pool.max} · up ${hub.up.count}/${hub.up.cap}` : "waiting for the run list";

export function Runs({ hub, selected, onSelect }: { hub: HubList | null; selected: string | null; onSelect: (id: string) => void }) {
	return (
		<Panel title="Runs" actions={<span className="muted">{poolUsage(hub)}</span>}>
			{hub ? <RunList hub={hub} selected={selected} onSelect={onSelect} /> : null}
		</Panel>
	);
}

function RunList({ hub, selected, onSelect }: { hub: HubList; selected: string | null; onSelect: (id: string) => void }) {
	const groups = groupRuns(hub);
	const marks = { destroy: nextDestroy(hub.runs), stop: nextStop(hub.runs) };
	const row = (r: RunItem) => <RunRow key={r.id} run={r} selected={r.id === selected} marks={marks} onSelect={onSelect} />;
	return (
		<>
			{hub.dockerDown ? <p className="needs bad">{label("wait", "docker-unavailable")} — the tick does not start the queue</p> : null}
			{hub.tickError ? <p className="needs bad">tick error · {hub.tickError}</p> : null}
			{hub.runs.length === 0 && hub.orphans.length === 0 ? (
				<div className="empty">
					<p className="muted">No runs yet. Submit one from the consumer repository:</p>
					<pre className="console">{SUBMIT_HINT}</pre>
				</div>
			) : null}
			{groups.pending.length > 0 ? <h3>Queued and active</h3> : null}
			<ul className="runs">{groups.pending.map(row)}</ul>
			{groups.pool.length > 0 ? <h3>Kept</h3> : null}
			<ul className="runs">{groups.pool.map(row)}</ul>
			{groups.ended.length > 0 ? <h3>Ended</h3> : null}
			<ul className="runs">{groups.ended.map(row)}</ul>
			{hub.orphans.length > 0 ? <h3>Orphans</h3> : null}
			<ul className="runs">
				{hub.orphans.map((o) => (
					<li key={o.project} className="run orphan">
						<code>{o.project}</code>
						<span className="axes">
							<Badge tone="bad">orphan</Badge>
							<Badge tone={o.running ? "warn" : "idle"}>{o.running ? "up" : "stopped"}</Badge>
						</span>
						<span className="muted">no hub record · clean up by hand</span>
					</li>
				))}
			</ul>
		</>
	);
}

type Marks = { destroy: string | null; stop: string | null };

function RunRow({ run, selected, marks, onSelect }: { run: RunItem; selected: boolean; marks: Marks; onSelect: (id: string) => void }) {
	return (
		<li>
			<button type="button" className={selected ? "run selected" : "run"} onClick={() => onSelect(run.id)}>
				<code>{run.id}</code>
				<span className="axes">
					<Badge tone={phaseTone(run)}>{run.phaseLabel}</Badge>
					{run.verdictLabel ? <Badge tone={verdictTone(run.verdict)}>verdict {run.verdictLabel}</Badge> : null}
					{run.keep ? <Badge tone="info">keep</Badge> : null}
					{marks.destroy === run.id ? <Badge tone="warn">next to destroy</Badge> : null}
					{marks.stop === run.id ? <Badge tone="warn">next to stop</Badge> : null}
				</span>
				<span className="muted">{rowDetail(run)}</span>
			</button>
		</li>
	);
}

export function Home({ hub }: { hub: HubList | null }) {
	return (
		<Panel title="No run selected">
			<p>{poolUsage(hub)}</p>
			<p className="muted">Pick a run on the left, or submit one from the consumer repository:</p>
			<pre className="console">{SUBMIT_HINT}</pre>
		</Panel>
	);
}
