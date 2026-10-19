import { fetchFlow, usePoll } from "../api";
import type { FlowPick } from "../nav";
import { codeTarget, pickedStep, STATUS_TONES, STEP_KIND_LABELS, statusLabel, stepFacts } from "../shared/flowTable";
import type { FlowSummary, FlowView as View } from "../shared/types";
import { Badge, Panel } from "../ui";
import { CodeGap, CodeView } from "./CodeView";
import { SOURCE_BADGES } from "./sourceBadge";

type Props = { id: string; version: string; flow: string; step: string | null; flows: FlowSummary[]; onPick: (pick: FlowPick) => void };

export function FlowView({ id, version, flow, step, flows, onPick }: Props) {
	const detail = usePoll(() => fetchFlow(id, flow), null, [id, flow, version]);
	const title = `Flow · ${flow}`;
	const close = (
		<button type="button" className="small" onClick={() => onPick({ flow: null, step: null })}>
			Close
		</button>
	);
	const view = detail.data;
	if (view === null) {
		return (
			<Panel title={title} actions={close}>
				{detail.error ? <p className="needs">Could not read the flow · {detail.error}</p> : <p className="muted">Loading</p>}
			</Panel>
		);
	}
	const active = pickedStep(view.flow, step);
	const target = codeTarget(view.flow, active);
	const badge = SOURCE_BADGES[view.source];
	return (
		<>
			<Panel
				title={title}
				actions={
					<span className="actions">
						<Badge tone={badge.tone}>{badge.label}</Badge>
						{close}
					</span>
				}
			>
				<FlowDetail view={view} active={active?.id ?? null} onStep={(next) => onPick({ flow, step: next })} />
			</Panel>
			{"gap" in target ? (
				<Panel title="Code">
					<CodeGap sentence={target.gap} />
				</Panel>
			) : (
				<CodeView key={target.file} id={id} flow={flow} flows={flows} target={target} version={version} />
			)}
		</>
	);
}

type DetailProps = { view: View; active: string | null; onStep: (step: string | null) => void };

export function FlowDetail({ view, active, onStep }: DetailProps) {
	const { flow } = view;
	const flowFile = flow.source?.file ?? null;
	return (
		<>
			<p>
				<Badge tone={STATUS_TONES[flow.status]}>{statusLabel(flow.status)}</Badge>
				{flow.title}
			</p>
			<h3>Steps — pick one to jump to its line</h3>
			{flow.stepList.length === 0 ? <p className="muted">This report has no step list</p> : null}
			<ol className="step-list">
				{flow.stepList.map((s) => {
					const on = s.id === active;
					return (
						<li key={s.id}>
							<button type="button" className={on ? "step-row active" : "step-row"} aria-pressed={on} onClick={() => onStep(on ? null : s.id)}>
								<span className="step-head">
									<code>{s.id}</code>
									<span>{s.title}</span>
									<span className="muted">{STEP_KIND_LABELS[s.kind]}</span>
									<Badge tone={STATUS_TONES[s.status]}>{statusLabel(s.status)}</Badge>
									<span className="muted">{stepFacts(s)}</span>
									<span className="muted">{s.source ? `${s.source.file === flowFile ? "" : `${s.source.file} `}line ${s.source.line}` : "no line"}</span>
								</span>
								{s.message ? <span className={s.status === "failed" ? "needs bad wrap" : "needs wrap"}>{s.message}</span> : null}
							</button>
						</li>
					);
				})}
			</ol>
		</>
	);
}
