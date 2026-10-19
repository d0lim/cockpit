import type { DataSource, Tone } from "../shared/types";

type SourceBadge = { tone: Tone; label: string };

export const SOURCE_BADGES: Readonly<Record<DataSource, SourceBadge>> = {
	live: { tone: "ok", label: "run tree" },
	bundle: { tone: "info", label: "bundle copy" },
};

const isDataSource = (source: string): source is DataSource => source === "live" || source === "bundle";

export const sourceBadge = (source: string): SourceBadge => (isDataSource(source) ? SOURCE_BADGES[source] : { tone: "idle", label: source });
