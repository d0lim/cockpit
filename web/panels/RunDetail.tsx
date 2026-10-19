import { Fragment } from "react";
import { fetchKit, fetchRun, usePoll } from "../api";
import { phaseTone, verdictTone } from "../runs";
import { label, stageLabel } from "../shared/labels";
import { served, UNSERVED_NOTICE } from "../shared/tabs";
import type { HubList, KitView, RunDetail as Detail, RunItem, Signal } from "../shared/types";
import { Badge, Copy, Notice, Panel, stamp } from "../ui";
import { RunActions } from "./RunActions";
import { RunServices } from "./Services";

type SideProps = { id: string; run: RunItem | undefined; hub: HubList | null; signals: Signal[]; onSelect: (id: string) => void };

export function RunSide({ id, run, hub, signals, onSelect }: SideProps) {
	const version = run ? JSON.stringify(run) : "";
	const detail = usePoll(() => fetchRun(id), null, [id, version]);
	const kit = usePoll(() => fetchKit(id), null, [id, version]);
	const ports = detail.data?.services ?? (detail.error ? [] : null);
	return (
		<>
			<RunDetail run={run ?? detail.data ?? undefined} hub={hub} detail={detail.data} error={detail.error} kit={kit.data} onSelect={onSelect} />
			<RunServices id={id} ports={ports} signals={signals} />
		</>
	);
}

type Props = { run: RunItem | undefined; hub: HubList | null; detail: Detail | null; error: string | null; kit: KitView | null; onSelect: (id: string) => void };

export function RunDetail({ run, hub, detail, error, kit, onSelect }: Props) {
	if (!run) {
		return (
			<Panel title="Run">
				<p className="muted">{error ?? "Loading the run"}</p>
			</Panel>
		);
	}
	return (
		<Panel title="Run">
			<p>
				<code>{run.id}</code>
			</p>
			<div className="axes">
				<span className="muted">lifecycle</span>
				<Badge tone={phaseTone(run)}>{run.phaseLabel}</Badge>
				<span className="muted">verdict</span>
				<Badge tone={verdictTone(run.verdict)}>{run.verdictLabel ?? "none yet"}</Badge>
				{run.keep ? <Badge tone="info">keep</Badge> : null}
			</div>
			<dl className="kv">
				<dt>commit</dt>
				<dd>
					<Copy text={run.sha}>{run.sha.slice(0, 12)}</Copy>
				</dd>
				<dt>suite</dt>
				<dd>{run.suite}</dd>
				<dt>host API</dt>
				<dd>
					{run.hostApi} · cockpit {run.cockpitVersion}
				</dd>
				<dt>submitter</dt>
				<dd>{run.submitter}</dd>
				<dt>submitted</dt>
				<dd>{stamp(run.createdAt)}</dd>
				{run.finishedAt ? (
					<>
						<dt>finished</dt>
						<dd>{stamp(run.finishedAt)}</dd>
					</>
				) : null}
				{run.phase === "queued" ? (
					<>
						<dt>queue</dt>
						<dd>
							#{run.position ?? "?"} · {run.waitReason ? label("wait", run.waitReason) : "starting soon"}
						</dd>
					</>
				) : null}
				{run.stopReason ? (
					<>
						<dt>stop reason</dt>
						<dd>{label("stop", run.stopReason)}</dd>
					</>
				) : null}
				{run.destroyReason ? (
					<>
						<dt>destroy reason</dt>
						<dd>{label("destroy", run.destroyReason)}</dd>
					</>
				) : null}
				{run.failedStage ? (
					<>
						<dt>stopped at stage</dt>
						<dd>{stageLabel(run.failedStage)}</dd>
					</>
				) : null}
				{run.notice ? (
					<>
						<dt>last failure</dt>
						<dd className="wrap">{run.notice}</dd>
					</>
				) : null}
			</dl>
			{served(run, hub) ? null : <Notice>{UNSERVED_NOTICE(run.hostApi)}</Notice>}
			{run.waitReason === "pool-all-kept" ? <Notice>Every kept environment has keep on. Release keep on one kept run to start.</Notice> : null}
			{run.recreate ? <Notice>The containers are gone; resume recreates them from the volumes.</Notice> : null}
			{run.redaction?.status === "failed" ? <Notice>The redaction rescan discarded the report bundle.</Notice> : null}
			<RunActions run={run} allowed={detail?.allowedEvents ?? null} onSelect={onSelect} />
			<Kit kit={kit} />
		</Panel>
	);
}

export function Kit({ kit }: { kit: KitView | null }) {
	if (kit === null) return null;
	return (
		<>
			<h3>Kit</h3>
			{kit.kit ? (
				<dl className="kv">
					<dt>kit directory</dt>
					<dd>
						<Copy text={kit.kit.dir} />
					</dd>
					<dt>README</dt>
					<dd>
						<Copy text={kit.kit.readme} />
					</dd>
				</dl>
			) : (
				<p className="muted">The kit exists only while the environment does.</p>
			)}
			{kit.exec ? (
				<>
					<div className="oneliner">
						<pre className="console">{kit.exec.command}</pre>
						<Copy text={kit.exec.command}>copy the exec command</Copy>
					</div>
					{kit.exec.flows.length > 0 ? (
						<p>
							<span className="muted">flows that can run again </span>
							{kit.exec.flows.map((id) => (
								<Fragment key={id}>
									{" "}
									<code>{id}</code>
								</Fragment>
							))}
						</p>
					) : (
						<p className="muted">The manifest has not been read yet.</p>
					)}
				</>
			) : null}
			<dl className="kv">
				<dt>report bundle</dt>
				<dd>
					<Copy text={kit.reportDir} />
				</dd>
			</dl>
		</>
	);
}
