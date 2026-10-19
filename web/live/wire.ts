import type { StreamKind } from "../store";

export const PROTOCOL = 1;
export const WORKER_URL = "/live/worker.js";
export const WORKER_NAME = `cockpit-live-${PROTOCOL}`;
export const HUB_TARGET = "hub";

export type FrameWire = { kind: "frame"; target: string; event: StreamKind; id: string; data: string };
export type StateWire = { kind: "state"; target: string; state: "open" | "lost" };
export type Wire = FrameWire | StateWire;
export type Replay = { kind: "replay"; wires: Wire[] };
export type Request = { kind: "sub" | "unsub"; target: string } | { kind: "bye" } | { kind: "ping" };

export const eventsUrl = (target: string) => `/api/events?target=${encodeURIComponent(target)}`;
