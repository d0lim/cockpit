import { label } from "./labels";
import type { FlowOutcome, FlowStep, FlowSummary, ManifestSource, NotProven, StepKind, StepStatus, Tone } from "./types";

export const FLOW_COLUMNS = ["Stage", "Flow", "Status", "Steps (passed/total)", "Failed", "Skipped", "Stopped at", "Title", "Requires", "Once per environment"] as const;

export const STATUS_AT = FLOW_COLUMNS.indexOf("Status");
export const FLOW_AT = FLOW_COLUMNS.indexOf("Flow");

export const statusLabel = (status: FlowOutcome | StepStatus | "notProven"): string => label("flowStatus", status);

export function flowCells(f: FlowSummary): string[] {
	const counts = [f.stage, f.id, statusLabel(f.status), `${f.passed}/${f.steps}`, String(f.failed), String(f.skipped), f.stoppedAt ?? "-"];
	return [...counts, f.title, f.requires.length === 0 ? "-" : f.requires.join(" · "), f.once ? "once" : "repeatable"];
}

export const rowClassOf = (status: FlowOutcome): "failed" | undefined => (status === "passed" ? undefined : "failed");

export function notProvenItems(notProven: NotProven | null | undefined): string[] {
	if (!notProven) return [];
	return [
		...notProven.skipped.map((s) => `skipped step · ${s}`),
		...notProven.declared.map((d) => `declared by the suite · ${d} · ${statusLabel("notProven")}`),
		...(notProven.unread ?? []).map((u) => `unread record · ${u}`),
	];
}

export const STATUS_TONES: Readonly<Record<FlowOutcome | StepStatus, Tone>> = {
	passed: "ok",
	failed: "bad",
	skipped: "warn",
	incomplete: "warn",
	running: "warn",
	notRun: "idle",
};

export const STEP_KIND_LABELS: Readonly<Record<StepKind, string>> = { request: "request", wait: "wait", capture: "capture" };

export const pickedStep = (flow: Pick<FlowSummary, "stepList">, stepId: string | null): FlowStep | null => flow.stepList.find((s) => s.id === stepId) ?? null;

export function stepFacts(step: Pick<FlowStep, "requests" | "durationMs" | "httpStatus">): string {
	const facts = [`requests ${step.requests}`, step.durationMs === undefined ? null : `${step.durationMs}ms`, step.httpStatus === undefined ? null : `HTTP ${step.httpStatus}`];
	return facts.filter((fact) => fact !== null).join(" · ");
}

export type FlowRange = { flow: string; start: number; end: number };

type Located = { id: string; source?: ManifestSource };

export function flowRanges(flows: readonly Located[], file: string, lines: number): FlowRange[] {
	const starts = flows
		.flatMap((f) => (f.source !== undefined && f.source.file === file && f.source.line <= lines ? [{ flow: f.id, start: f.source.line }] : []))
		.sort((a, b) => a.start - b.start);
	return starts.map((s, i) => ({ ...s, end: Math.max(s.start, (starts[i + 1]?.start ?? lines + 1) - 1) }));
}

export const highlightRange = (ranges: readonly FlowRange[], flow: string): FlowRange | null => ranges.find((r) => r.flow === flow) ?? null;

export const rangeText = (range: FlowRange): string => `flow range lines ${range.start}-${range.end}`;

export type CodeTarget = { file: string; line: number; focus: boolean };
export type NoCode = { gap: string };

export const NO_SOURCE = "this run's manifest has no source location";

export function codeTarget(flow: Pick<FlowSummary, "source">, step: Pick<FlowStep, "source"> | null): CodeTarget | NoCode {
	if (step?.source) return { file: step.source.file, line: step.source.line, focus: true };
	return flow.source ? { file: flow.source.file, line: flow.source.line, focus: false } : { gap: NO_SOURCE };
}

export const jumpLine = (target: CodeTarget, view: { file: string; lines: number }): number | null =>
	view.file === target.file && target.line >= 1 && target.line <= view.lines ? target.line : null;
