import { label } from "../shared/labels";
import type { CollectorState, Tone } from "../shared/types";
import { Badge, stamp } from "../ui";

const TONES: Readonly<Record<string, Tone>> = { armed: "ok", backoff: "warn", stopped: "idle", "report-only": "info" };

export function collectorLine(c: CollectorState): string {
	const parts = [c.id, label("collectors", c.state), `last success ${stamp(c.lastOkAt)}`, `failures ${c.failures}`];
	if (c.reason) parts.push(`reason ${c.reason}`);
	return parts.join(" · ");
}

export function CollectorStatus({ collectors }: { collectors: readonly CollectorState[] }) {
	if (collectors.length === 0) return null;
	return (
		<ul className="collectors" aria-label="Collector status">
			{collectors.map((c) => (
				<li key={c.id} className="collector">
					<Badge tone={TONES[c.state] ?? "idle"}>{label("collectors", c.state)}</Badge>
					<span>{collectorLine(c)}</span>
				</li>
			))}
		</ul>
	);
}
