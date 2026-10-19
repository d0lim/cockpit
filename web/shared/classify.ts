import type { Tone } from "./types";

const TONES: ReadonlySet<string> = new Set(["ok", "warn", "bad", "idle", "info"]);

export const NEUTRAL = "neutral";

export function toneOf(value: unknown): Tone | typeof NEUTRAL {
	return typeof value === "string" && TONES.has(value) ? (value as Tone) : NEUTRAL;
}

export function serviceTone(status: string, health: string | undefined): Tone {
	if (status === "running") {
		if (health === "starting") return "warn";
		return health === undefined || health === "" || health === "healthy" ? "ok" : "bad";
	}
	if (status === "absent") return "idle";
	if (status === "exited") return "idle";
	return "bad";
}
