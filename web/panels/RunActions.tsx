import { useState } from "react";
import { ApiFailure, operate } from "../api";
import { confirmText, type DangerOp, needsConfirm, needsForce } from "../runs";
import { keepFrozen } from "../shared/pool";
import type { Candidate, RunEvent, RunItem, RunOp } from "../shared/types";
import { stamp } from "../ui";

const LABEL: Record<RunOp, string> = { keep: "Keep", unkeep: "Release keep", stop: "Stop", resume: "Resume", destroy: "Destroy", abort: "Abort" };
const DONE: Record<RunOp, string> = {
	keep: "keep is on",
	unkeep: "keep is released",
	stop: "stopped",
	resume: "resumed",
	destroy: "destroyed",
	abort: "abort signal sent",
};

type Shown = { ok: boolean; text: string; candidates?: Candidate[] };

function failureOf(e: Error): Shown {
	if (!(e instanceof ApiFailure)) return { ok: false, text: e.message };
	const text = `${e.code} · ${e.message}`;
	if (e.code !== "CONFLICT_CAP") return { ok: false, text };
	return { ok: false, text, candidates: Array.isArray(e.body.candidates) ? (e.body.candidates as Candidate[]) : [] };
}

type Props = { run: RunItem; allowed: readonly RunEvent[] | null; onSelect: (id: string) => void };

export function RunActions({ run, allowed, onSelect }: Props) {
	const [confirm, setConfirm] = useState<DangerOp | null>(null);
	const [busy, setBusy] = useState<RunOp | null>(null);
	const [shown, setShown] = useState<Shown | null>(null);
	const idle = busy === null;
	const can = (event: RunEvent) => idle && (allowed?.includes(event) ?? false);
	const keepable = idle && !keepFrozen(run);

	const act = (op: RunOp) => {
		setConfirm(null);
		setBusy(op);
		setShown(null);
		operate(run.id, op, needsForce(op, run))
			.then(({ run: after }) => setShown({ ok: true, text: `${DONE[op]} · now ${after.phaseLabel}` }))
			.catch((e: Error) => setShown(failureOf(e)))
			.finally(() => setBusy(null));
	};
	const ask = (op: DangerOp) => {
		if (!needsConfirm(op, run)) return act(op);
		setShown(null);
		setConfirm(op);
	};

	return (
		<div className="run-actions">
			<div className="actions">
				{run.keep ? (
					<button type="button" className="small" disabled={!keepable} onClick={() => ask("unkeep")}>
						Release keep
					</button>
				) : (
					<button type="button" className="small" disabled={!keepable} onClick={() => act("keep")}>
						Keep
					</button>
				)}
				<button type="button" className="small" disabled={!can("stop")} onClick={() => ask("stop")}>
					Stop…
				</button>
				<button type="button" className="small" disabled={!can("resume")} onClick={() => act("resume")}>
					Resume
				</button>
				<button type="button" className="small danger" disabled={!can("destroy")} onClick={() => ask("destroy")}>
					Destroy…
				</button>
				<button type="button" className="small" disabled={!can("abort")} onClick={() => ask("abort")}>
					Abort…
				</button>
			</div>
			{confirm ? (
				<div className="confirm">
					<span>{confirmText(confirm, run)}</span>
					<button type="button" className="small danger" onClick={() => act(confirm)}>
						{LABEL[confirm]}
					</button>
					<button type="button" className="small" onClick={() => setConfirm(null)}>
						Cancel
					</button>
				</div>
			) : null}
			{busy ? <p className="running">{LABEL[busy]} requested. The result shows here when it is done.</p> : null}
			{shown ? <Outcome shown={shown} onSelect={onSelect} /> : null}
		</div>
	);
}

function Outcome({ shown, onSelect }: { shown: Shown; onSelect: (id: string) => void }) {
	const candidates = shown.candidates;
	return (
		<div className={shown.ok ? "result ok" : "result bad"}>
			<span>{shown.text}</span>
			{candidates?.length === 0 ? <p>Active runs hold the running slots.</p> : null}
			{candidates && candidates.length > 0 ? (
				<>
					<p>Runs to stop first — oldest last up first.</p>
					<ol className="candidates">
						{candidates.map((c) => (
							<li key={c.id}>
								<button type="button" className="link" onClick={() => onSelect(c.id)}>
									{c.id}
								</button>
								<span className="muted">
									{" "}
									· last up {stamp(c.lastUpAt || c.finishedAt)} · finished {stamp(c.finishedAt)}
									{c.keep ? " · keep" : ""}
								</span>
							</li>
						))}
					</ol>
				</>
			) : null}
		</div>
	);
}
