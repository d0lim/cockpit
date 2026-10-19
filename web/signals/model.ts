import type { CollectorState, RunItem, Signal } from "../shared/types";

export const LOG_COLLECTOR = "logs";
export const SUPERVISOR_COLLECTOR = "supervisor";
export const SERVICES_TABLE = "services";
export const CONTAINER_ATTR = "container";
export const RESERVED = ["table", "status", "label", "tone"] as const;

const revKey = (s: Signal) => `${s.collector}\u0000${s.kind}\u0000${s.id}${s.span ? `\u0000${s.span.traceId}` : ""}`;

export function latestRevisions(signals: readonly Signal[]): Signal[] {
	const winners = new Map<string, Signal>();
	for (const s of signals) {
		if (s.id === undefined || s.id === "" || s.kind === "metric") continue;
		const key = revKey(s);
		const cur = winners.get(key);
		if (cur === undefined || (s.rev ?? 0) > (cur.rev ?? 0) || ((s.rev ?? 0) === (cur.rev ?? 0) && s.seq > cur.seq)) winners.set(key, s);
	}
	return signals.filter((s) => s.id === undefined || s.id === "" || s.kind === "metric" || winners.get(revKey(s)) === s);
}

export type Placement = "in-bundle" | "after-bundle" | "live";

export function placement(seq: number, frozen: boolean, frozenSeq: number): Placement {
	if (!frozen) return "live";
	return seq <= frozenSeq ? "in-bundle" : "after-bundle";
}

export const PLACEMENT_LABELS: Readonly<Record<Placement, string>> = { "in-bundle": "in bundle", "after-bundle": "after the bundle", live: "" };

export const FROZEN_DIVIDER = "After the report was finalized";

export function droppedLine(dropped: Readonly<Record<string, number>>): string | null {
	const entries = Object.entries(dropped).filter(([, n]) => n > 0);
	const total = entries.reduce((sum, [, n]) => sum + n, 0);
	if (total === 0) return null;
	return `${total} older records dropped (${entries.map(([reason, n]) => `${reason} ${n}`).join(", ")})`;
}

export const holeNotice = (from: number, to: number) =>
	`Signals ${from}-${to} were dropped before this screen read them; reading continues after them.`;

export const attrText = (v: unknown): string => (v === undefined ? "" : String(v));

export type EmptyKind = "before-boot" | "env-off" | "no-collectors" | "none-of-kind" | "collectors-stopped" | "no-signals";

export const EMPTY_SENTENCES: Readonly<Record<EmptyKind, string>> = {
	"before-boot": "The environment has not booted yet. Signals start once it is up.",
	"env-off": "The environment is off. Only the stored signals are shown.",
	"no-collectors": "This run declares no collectors, so only hub signals appear here.",
	"none-of-kind": "This run has signals, but none of the kind this tab shows.",
	"collectors-stopped": "Collectors are stopped, so nothing new arrives until they are armed again.",
	"no-signals": "No signals yet. Collectors are armed and nothing has arrived.",
};

const PRE_BOOT = new Set(["queued", "preparing", "building", "sealing", "booting"]);

export function emptyKind(run: Pick<RunItem, "phase" | "env">, collectors: readonly CollectorState[], count: number, total = count): EmptyKind | null {
	if (count > 0) return run.env === "up" || PRE_BOOT.has(run.phase) ? null : "env-off";
	if (PRE_BOOT.has(run.phase)) return "before-boot";
	if (run.env !== "up") return "env-off";
	if (collectors.length === 0) return "no-collectors";
	if (total > 0) return "none-of-kind";
	if (collectors.every((c) => c.state === "stopped")) return "collectors-stopped";
	return "no-signals";
}

export type Item<T> = { kind: "row"; row: T } | { kind: "divider" } | { kind: "dropped"; text: string };

export function withBoundaries<T extends { seq: number }>(rows: readonly T[], meta: { frozen: boolean; frozenSeq: number; dropped: Readonly<Record<string, number>> }): Item<T>[] {
	const drop = droppedLine(meta.dropped);
	const tailHead: Item<T>[] = drop === null ? [] : [{ kind: "dropped", text: drop }];
	if (!meta.frozen) return [...tailHead, ...rows.map((row): Item<T> => ({ kind: "row", row }))];
	const before = rows.filter((r) => r.seq <= meta.frozenSeq).map((row): Item<T> => ({ kind: "row", row }));
	const after = rows.filter((r) => r.seq > meta.frozenSeq).map((row): Item<T> => ({ kind: "row", row }));
	return [...before, { kind: "divider" }, ...tailHead, ...after];
}
