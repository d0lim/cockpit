import { label } from "./shared/labels";
import { inPool, isActive, isLive, isPending } from "./shared/pool";
import type { HubList, RunItem, RunOp, Tone, Verdict } from "./shared/types";
import { stamp } from "./ui";

export const UI_SUBMITTER = "ui";
export const SUBMIT_HINT = "cockpit runs submit";

export const queryable = (r: RunItem): boolean => isLive(r);

export type RunGroups = { pending: RunItem[]; pool: RunItem[]; ended: RunItem[] };

const finishedKey = (r: RunItem) => r.finishedAt ?? r.createdAt;

export function groupRuns(list: HubList): RunGroups {
	const recent = (key: (r: RunItem) => string) => (a: RunItem, b: RunItem) => key(b).localeCompare(key(a));
	return {
		pending: list.runs.filter(isPending).sort((a, b) => (a.position ?? 0) - (b.position ?? 0)),
		pool: list.runs.filter(inPool).sort(recent(finishedKey)),
		ended: list.runs.filter((r) => !isPending(r) && !inPool(r)).sort(recent((r) => r.createdAt)),
	};
}

export function phaseTone(r: RunItem): Tone {
	if (r.mark !== undefined) return "bad";
	if (isPending(r)) return r.phase === "queued" ? "idle" : "warn";
	if (r.phase !== "done") return "warn";
	return r.env === "up" ? "ok" : r.env === "stopped" ? "info" : "idle";
}

const VERDICT_TONES: Readonly<Record<Verdict, Tone>> = { passed: "ok", failed: "bad", error: "bad", aborted: "warn" };
export const verdictTone = (verdict: Verdict | null): Tone => (verdict === null ? "idle" : VERDICT_TONES[verdict]);

export type DangerOp = Extract<RunOp, "stop" | "destroy" | "abort" | "unkeep">;

export const needsConfirm = (op: DangerOp, r: RunItem) => op !== "unkeep" || r.submitter !== UI_SUBMITTER;
export const needsForce = (op: RunOp, r: RunItem) => (op === "stop" || op === "destroy" || op === "unkeep") && r.submitter !== UI_SUBMITTER;

export function confirmText(op: DangerOp, r: RunItem): string {
	const owner = r.submitter === UI_SUBMITTER ? "" : ` This run was submitted by ${r.submitter}.`;
	return `${r.id} · ${r.sha.slice(0, 7)} · ${r.suite} — ${consequence(op, r)}${owner}`;
}

function consequence(op: DangerOp, r: RunItem): string {
	if (op === "stop") return "In-memory state of the stand-ins is lost, and resume works only while the running environments are under the cap.";
	if (op === "abort") return "Stops the running suite. The environment is stopped like a finished run and its data stays.";
	if (op === "unkeep") return "Releases keep. The run becomes a candidate for the kept pool cleanup.";
	if (r.phase === "queued") return "Removes it from the queue.";
	if (isActive(r)) return "Stops the run and removes the environment. An aborted report remains.";
	return `Removes the volumes too. Only the report remains.${r.keep ? " This environment has keep on." : ""}`;
}

export function rowDetail(r: RunItem): string {
	const head = `${r.suite} · ${r.sha.slice(0, 7)}`;
	if (r.phase === "queued") return `queued #${r.position ?? "?"} · ${r.waitReason ? label("wait", r.waitReason) : "starting soon"} · ${head}`;
	if (isActive(r)) return `${head} · started ${stamp(r.startedAt)}`;
	const base = `${head} · by ${r.submitter} · finished ${stamp(r.finishedAt)}`;
	if (r.env === "stopped" && r.stopReason) return `${base} · ${label("stop", r.stopReason)}`;
	if (r.env === "destroyed") return `${base} · only the report remains${r.destroyReason ? ` · ${label("destroy", r.destroyReason)}` : ""}`;
	return base;
}
