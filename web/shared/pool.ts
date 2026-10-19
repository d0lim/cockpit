import type { Phase, RunItem } from "./types";

const ACTIVE: ReadonlySet<Phase> = new Set(["preparing", "building", "sealing", "booting", "testing", "reporting"]);
const PRE_BOOT: ReadonlySet<Phase> = new Set(["queued", "preparing", "building", "sealing"]);

type Lifecycle = Pick<RunItem, "mark" | "phase">;
type Placed = Pick<RunItem, "mark" | "phase" | "env">;

export const isActive = (r: Lifecycle): boolean => r.mark === undefined && ACTIVE.has(r.phase);
export const isPending = (r: Lifecycle): boolean => r.phase === "queued" || isActive(r);
export const isPreBoot = (r: Lifecycle): boolean => r.mark === undefined && PRE_BOOT.has(r.phase);

export const inPool = (r: Placed): boolean =>
	r.mark === undefined && (r.phase === "done" || r.phase === "stopping" || r.phase === "resuming") && (r.env === "up" || r.env === "stopped");

export const stoppable = (r: Placed): boolean => r.mark === undefined && r.phase === "done" && r.env === "up";

export const keepFrozen = (r: Placed): boolean =>
	r.mark !== undefined || r.phase === "destroying" || (r.phase === "done" && (r.env === "absent" || r.env === "destroyed"));

export const isLive = (r: Pick<RunItem, "mark" | "env" | "slot">): boolean => r.mark === undefined && r.env === "up" && r.slot !== undefined;

type Finished = Pick<RunItem, "id" | "createdAt" | "finishedAt">;
type LastUp = Pick<RunItem, "id" | "finishedAt" | "lastUpAt">;
const at = (value: string | undefined) => value ?? "";
export const byFinished = (a: Finished, b: Finished) => at(a.finishedAt ?? a.createdAt).localeCompare(at(b.finishedAt ?? b.createdAt)) || a.id.localeCompare(b.id);
export const byLastUp = (a: LastUp, b: LastUp) => at(a.lastUpAt ?? a.finishedAt).localeCompare(at(b.lastUpAt ?? b.finishedAt)) || a.id.localeCompare(b.id);

export const nextDestroy = (runs: RunItem[]) => runs.filter((r) => inPool(r) && r.phase === "done" && !r.keep).sort(byFinished)[0]?.id ?? null;
export const nextStop = (runs: RunItem[]) => runs.filter(stoppable).sort(byLastUp)[0]?.id ?? null;
