import type { Open } from "../live/fanout";
import type { Source } from "../live/source";
import { openSources } from "../live/source";

export class FakeSource implements Source {
	onopen: ((ev: Event) => unknown) | null = null;
	onerror: ((ev: Event) => unknown) | null = null;
	listeners = new Map<string, (msg: MessageEvent) => void>();
	closed = false;
	constructor(readonly url: string) {}
	addEventListener(kind: string, fn: (msg: MessageEvent) => void) {
		this.listeners.set(kind, fn);
	}
	close() {
		this.closed = true;
	}
	emit(kind: string, id: string, data: unknown) {
		this.listeners.get(kind)?.(new MessageEvent(kind, { data: typeof data === "string" ? data : JSON.stringify(data), lastEventId: id }));
	}
}

export class Sources {
	all: FakeSource[] = [];
	readonly open: Open = openSources((url) => {
		const s = new FakeSource(url);
		this.all.push(s);
		return s;
	});
	live(target: string): FakeSource[] {
		return this.all.filter((s) => !s.closed && s.url.endsWith(`target=${encodeURIComponent(target)}`));
	}
	opened(target: string): number {
		return this.all.filter((s) => s.url.endsWith(`target=${encodeURIComponent(target)}`)).length;
	}
	one(target: string): FakeSource {
		const open = this.live(target);
		if (open.length !== 1) throw new Error(`${open.length} open sources for ${target}`);
		return open[0] as FakeSource;
	}
}

export const sig = (seq: number) => ({ seq, ts: "2026-10-19T01:00:00Z", tsSource: "host", kind: "event", collector: "toy-db", attrs: {} });

export const settle = async (until: () => boolean, what: string) => {
	for (let i = 0; i < 400; i++) {
		if (until()) return;
		await new Promise((r) => setTimeout(r, 2));
	}
	throw new Error(`never settled: ${what}`);
};
