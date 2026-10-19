import type { Event, HubList, JobEvent } from "./api";

export type * from "./api";

export type RunOp = "keep" | "unkeep" | "stop" | "resume" | "destroy" | "abort";
export type RunEvent = Extract<Event, "stop" | "resume" | "destroy" | "abort">;
export type DataSource = "live" | "bundle";

export type FrameHead = { target: string; at: string };
export type RunsEvent = FrameHead & { kind: "runs"; list: HubList };
export type JobFrame = FrameHead & { kind: "job" } & JobEvent;
export type ResyncFrame = { target: string; from: number; to: number };
export type RefusedFrame = { target: string; reason: string };
