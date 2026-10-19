import { fetchReport, usePoll } from "../api";
import type { FlowPick } from "../nav";
import { verdictTone } from "../runs";
import { notProvenItems } from "../shared/flowTable";
import { stageLabel } from "../shared/labels";
import type { CollectStage, ReportView, RunItem } from "../shared/types";
import { Badge, Copy, Notice, Panel } from "../ui";
import { FlowView } from "./FlowDetail";
import { FlowTable } from "./FlowTable";

type Summary = { reasons?: string[]; seal?: string; stages?: Record<string, CollectStage> };
type Bundle = Extract<ReportView, { status: "bundle" }>;
type Picking = { pick: FlowPick; onPick: (pick: FlowPick) => void; version: string };

export function Report({ id, run, pick, onPick }: { id: string; run: RunItem | undefined } & Omit<Picking, "version">) {
	const version = run ? `${run.phase}|${run.env}|${run.verdict}|${run.redaction?.status ?? ""}` : "";
	const report = usePoll(() => fetchReport(id), null, [id, version]);
	if (report.error) return <p className="needs">Could not read the report · {report.error}</p>;
	if (report.data === null) return <p className="muted">Loading</p>;
	return <ReportBody id={id} rep={report.data} picking={{ pick, onPick, version }} />;
}

function Head({ rep }: { rep: ReportView }) {
	return (
		<div className="axes">
			<span className="muted">lifecycle</span>
			<Badge tone="idle">{rep.phaseLabel}</Badge>
			<span className="muted">verdict</span>
			<Badge tone={verdictTone(rep.verdict)}>{rep.verdictLabel ?? "none yet"}</Badge>
		</div>
	);
}

export function ReportBody({ id, rep, picking }: { id: string; rep: ReportView; picking: Picking }) {
	if (rep.status === "pending") {
		return (
			<Panel title="Report">
				<Head rep={rep} />
				<Notice>The report is written when the run finishes. Current stage: {rep.phaseLabel}.</Notice>
			</Panel>
		);
	}
	if (rep.status === "redaction-failed") {
		return (
			<Panel title="Report">
				<Head rep={rep} />
				<Notice>The redaction rescan discarded the bundle. The files it caught:</Notice>
				<ul className="files">
					{rep.files.map((f) => (
						<li key={f}>
							<code>{f}</code>
						</li>
					))}
				</ul>
			</Panel>
		);
	}
	if (rep.status === "meta-only") {
		const tail = rep.tail ?? [];
		return (
			<Panel title="Report">
				<Head rep={rep} />
				<Notice>Finished without a bundle. Stopped at stage {stageLabel(rep.failedStage)}.</Notice>
				{rep.notice ? <p className="wrap">reason · {rep.notice}</p> : null}
				<h3>Last lines of the failed stage</h3>
				{tail.length > 0 ? <pre className="console">{tail.join("\n")}</pre> : <p className="muted">No lines left</p>}
			</Panel>
		);
	}
	return <BundleReport id={id} rep={rep} summary={(rep.summary ?? {}) as Summary} picking={picking} />;
}

function BundleReport({ id, rep, summary, picking }: { id: string; rep: Bundle; summary: Summary; picking: Picking }) {
	const { steps, stoppedAt: first } = rep;
	const flows = rep.flows ?? [];
	const { pick, onPick, version } = picking;
	const failedStages = Object.entries(summary.stages ?? {}).filter(([, stage]) => stage.status === "error");
	const items = notProvenItems(rep.notProven);
	return (
		<>
			<Panel title="Report summary">
				<Head rep={rep} />
				<p>
					steps {steps?.actual ?? "-"}/{steps?.expected ?? "-"} · failed {steps?.failed ?? "-"} · skipped {steps?.skipped ?? "-"} · requests {steps?.requests ?? "-"} · seal{" "}
					{summary.seal ?? "-"}
				</p>
				{first ? (
					<div className="first-failure">
						<h3>Stopped at</h3>
						<p>
							<code>{first.flow}</code> · <code>{first.step}</code>
						</p>
						{first.message ? <pre className="console">{first.message}</pre> : null}
					</div>
				) : null}
				{summary.reasons && summary.reasons.length > 0 ? (
					<ul className="reasons">
						{summary.reasons.map((r) => (
							<li key={r}>{r}</li>
						))}
					</ul>
				) : null}
				<h3>Not proven</h3>
				{items.length === 0 ? (
					<p className="muted">None</p>
				) : (
					<ul className="reasons">
						{items.map((item) => (
							<li key={item}>{item}</li>
						))}
					</ul>
				)}
			</Panel>
			<Panel title="Flow results">
				<FlowTable flows={flows} selected={pick.flow} onSelect={(flow) => onPick({ flow, step: null })} />
				{failedStages.map(([name, stage]) => (
					<p key={name} className="needs">
						collection failed · {name} {stage.detail ?? ""}
					</p>
				))}
			</Panel>
			{pick.flow === null ? null : <FlowView key={pick.flow} id={id} version={version} flow={pick.flow} step={pick.step} flows={flows} onPick={onPick} />}
			<Panel title="Bundle directory">
				<p>
					<Copy text={rep.reportDir} />
				</p>
				<p className="needs">Open the raw files (runner results, signals, sources) in this directory. The hub does not serve raw files.</p>
			</Panel>
		</>
	);
}
