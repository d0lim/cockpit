import type { Event, HubList } from "./api";

export type * from "./api";

export type RunOp = "keep" | "unkeep" | "stop" | "resume" | "destroy" | "abort";
export type RunEvent = Extract<Event, "stop" | "resume" | "destroy" | "abort">;
export type DataSource = "live" | "bundle";

export type FrameHead = { target: string; at: string };
export type RunsEvent = FrameHead & { kind: "runs"; list: HubList };
export type JobEvent = FrameHead & { kind: "job"; jobId: string; title: string; phase: "start" | "out" | "exit"; text?: string; code?: number };
export type ResyncFrame = { target: string; from: number; to: number };
export type RefusedFrame = { target: string; reason: string };
