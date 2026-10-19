import type { DataSource, Tone } from "../shared/types";

export const SOURCE_BADGES: Readonly<Record<DataSource, { tone: Tone; label: string }>> = {
	live: { tone: "ok", label: "run tree" },
	bundle: { tone: "info", label: "bundle copy" },
};
