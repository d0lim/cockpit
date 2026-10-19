import { useState } from "react";
import type { Signal } from "../shared/types";
import { attrText, CONTAINER_ATTR, LOG_COLLECTOR } from "../signals/model";
import { Panel, time } from "../ui";

export function logSources(signals: readonly Signal[]): Map<string, Signal[]> {
	const out = new Map<string, Signal[]>();
	for (const s of signals) {
		if (s.collector !== LOG_COLLECTOR || s.kind !== "event") continue;
		const source = attrText(s.attrs[CONTAINER_ATTR]) || "(unknown)";
		out.set(source, [...(out.get(source) ?? []), s]);
	}
	return out;
}

export function Logs({ signals, picked }: { signals: readonly Signal[]; picked?: string }) {
	const sources = logSources(signals);
	const names = [...sources.keys()];
	const [chosen, setChosen] = useState<string | null>(picked ?? null);
	const source = chosen !== null && sources.has(chosen) ? chosen : (names[0] ?? null);
	const lines = source === null ? [] : (sources.get(source) ?? []);
	return (
		<Panel
			title="Logs"
			actions={
				<select value={source ?? ""} onChange={(e) => setChosen(e.target.value)} aria-label="Log source">
					{names.map((s) => (
						<option key={s}>{s}</option>
					))}
				</select>
			}
		>
			{source === null ? <p className="muted">No container log lines yet.</p> : null}
			<pre className="console">{lines.map((l) => `${time(l.ts)} ${l.event?.body ?? ""}`).join("\n")}</pre>
		</Panel>
	);
}
