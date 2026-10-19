import { STREAM_KINDS } from "../store";
import type { Open } from "./fanout";
import { eventsUrl, type Wire } from "./wire";

export type Source = {
	addEventListener(kind: string, fn: (msg: MessageEvent) => void): void;
	onopen: ((ev: Event) => unknown) | null;
	onerror: ((ev: Event) => unknown) | null;
	close(): void;
};

export function listen(source: Source, target: string, emit: (w: Wire) => void): void {
	source.onopen = () => emit({ kind: "state", target, state: "open" });
	source.onerror = () => emit({ kind: "state", target, state: "lost" });
	for (const event of STREAM_KINDS) {
		source.addEventListener(event, (msg) => emit({ kind: "frame", target, event, id: msg.lastEventId, data: String(msg.data) }));
	}
}

export const openSources =
	(factory: (url: string) => Source): Open =>
	(target, emit) => {
		const source = factory(eventsUrl(target));
		listen(source, target, emit);
		return source;
	};
