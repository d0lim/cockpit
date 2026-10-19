import { admit, NO_SEEN, type Seen } from "./seen";
import type { StateWire, Wire } from "./wire";

export type Upstream = { close(): void };
export type Open = (target: string, emit: (w: Wire) => void) => Upstream;
export type Sink<S> = { live(subs: ReadonlySet<S>, w: Wire): void; replay(sub: S, wires: readonly Wire[]): void };

export const REPLAY_CAP = 25_000;

type Stream<S> = { up: Upstream | null; subs: Set<S>; log: Wire[]; seen: Seen; state: StateWire | null };

export class Fanout<S> {
	private readonly streams = new Map<string, Stream<S>>();

	constructor(
		private readonly open: Open,
		private readonly sink: Sink<S>,
		private readonly cap = REPLAY_CAP,
	) {}

	targets(): string[] {
		return [...this.streams.keys()];
	}

	subscribe(sub: S, target: string): void {
		let s = this.streams.get(target);
		if (s === undefined) {
			const fresh: Stream<S> = { up: null, subs: new Set(), log: [], seen: NO_SEEN, state: null };
			this.streams.set(target, fresh);
			fresh.up = this.open(target, (w) => this.take(target, fresh, w));
			s = fresh;
		}
		if (s.subs.has(sub)) return;
		s.subs.add(sub);
		this.sink.replay(sub, s.state === null ? s.log : [s.state, ...s.log]);
	}

	unsubscribe(sub: S, target: string): void {
		const s = this.streams.get(target);
		if (s === undefined || !s.subs.delete(sub) || s.subs.size > 0) return;
		this.streams.delete(target);
		s.up?.close();
	}

	drop(sub: S): void {
		for (const target of this.targets()) this.unsubscribe(sub, target);
	}

	close(): void {
		for (const s of this.streams.values()) s.up?.close();
		this.streams.clear();
	}

	private take(target: string, s: Stream<S>, w: Wire): void {
		if (this.streams.get(target) !== s) return;
		if (w.kind === "state") {
			s.state = w;
			this.sink.live(s.subs, w);
			return;
		}
		const next = admit(s.seen, w.event, w.id);
		if (next === null) return;
		if (s.seen.epoch !== "" && next.epoch !== s.seen.epoch) s.log = [];
		s.seen = next;
		if (w.event === "runs") {
			const i = s.log.findIndex((x) => x.kind === "frame" && x.event === "runs");
			if (i >= 0) s.log.splice(i, 1);
		}
		s.log.push(w);
		if (s.log.length > this.cap) s.log.splice(0, s.log.length - this.cap);
		this.sink.live(s.subs, w);
	}
}
