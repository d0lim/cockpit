import { parseCursor, type StreamKind } from "../store";

export type Seen = { epoch: string; bus: number; signal: number; refused: boolean };

export const NO_SEEN: Seen = { epoch: "", bus: 0, signal: 0, refused: false };

export function admit(seen: Seen, event: StreamKind, id: string): Seen | null {
	const c = parseCursor(id);
	if (c === null) return null;
	const base = seen.epoch === c.epoch ? seen : { ...NO_SEEN, epoch: c.epoch };
	switch (event) {
		case "runs":
		case "job":
			return c.bus > base.bus ? { ...base, bus: c.bus, signal: Math.max(base.signal, c.signal) } : null;
		case "signal":
		case "resync":
			return c.signal > base.signal ? { ...base, signal: c.signal, bus: Math.max(base.bus, c.bus) } : null;
		case "signal-refused":
			return base.refused ? null : { ...base, refused: true };
	}
	return null;
}
