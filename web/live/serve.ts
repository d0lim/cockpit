import { Fanout, type Open } from "./fanout";
import type { Replay, Request, Wire } from "./wire";

export const PING_MS = 10_000;
export const IDLE_MS = 35_000;

export type PortLike = { postMessage(m: Wire | Replay | Request): void; onmessage: ((e: MessageEvent) => void) | null; start?(): void };

export class WorkerHub {
	private readonly fan: Fanout<PortLike>;
	private readonly heard = new Map<PortLike, number>();

	constructor(
		open: Open,
		private readonly now: () => number = Date.now,
	) {
		this.fan = new Fanout<PortLike>(open, {
			live: (ports, w) => {
				for (const p of ports) p.postMessage(w);
			},
			replay: (p, wires) => {
				if (wires.length > 0) p.postMessage({ kind: "replay", wires: [...wires] });
			},
		});
	}

	connect(port: PortLike): void {
		this.heard.set(port, this.now());
		port.onmessage = (e) => this.handle(port, e.data as Request);
		port.start?.();
	}

	ports(): number {
		return this.heard.size;
	}

	sweep(idle = IDLE_MS): void {
		const now = this.now();
		for (const [port, at] of this.heard) if (now - at > idle) this.forget(port);
	}

	private forget(port: PortLike): void {
		this.heard.delete(port);
		this.fan.drop(port);
	}

	private handle(port: PortLike, r: Request): void {
		if (r.kind === "bye") {
			this.forget(port);
			return;
		}
		this.heard.set(port, this.now());
		if (r.kind === "sub") this.fan.subscribe(port, r.target);
		if (r.kind === "unsub") this.fan.unsubscribe(port, r.target);
	}
}
