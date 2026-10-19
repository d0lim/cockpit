import type { Action } from "../store";
import { admit, NO_SEEN, type Seen } from "./seen";
import type { Wire } from "./wire";

export type Push = (a: Action) => void;
export type Live = { subscribe(target: string, push: Push): () => void };

type Want = { pushes: Set<Push>; seen: Seen };

export class Router implements Live {
	private readonly wants = new Map<string, Want>();

	constructor(private readonly hooks: { first(target: string): void; last(target: string): void }) {}

	targets(): string[] {
		return [...this.wants.keys()];
	}

	subscribe(target: string, push: Push): () => void {
		let want = this.wants.get(target);
		const first = want === undefined;
		if (want === undefined) {
			want = { pushes: new Set(), seen: NO_SEEN };
			this.wants.set(target, want);
		}
		want.pushes.add(push);
		if (first) this.hooks.first(target);
		return () => {
			const w = this.wants.get(target);
			if (w === undefined || !w.pushes.delete(push) || w.pushes.size > 0) return;
			this.wants.delete(target);
			this.hooks.last(target);
		};
	}

	deliver = (w: Wire): void => {
		const want = this.wants.get(w.target);
		if (want === undefined) return;
		if (w.kind === "state") {
			for (const push of want.pushes) push({ kind: "connection", state: w.state });
			return;
		}
		const next = admit(want.seen, w.event, w.id);
		if (next === null) return;
		want.seen = next;
		let data: unknown;
		try {
			data = JSON.parse(w.data);
		} catch {
			return;
		}
		for (const push of want.pushes) push({ kind: "frame", event: w.event, id: w.id, data });
	};
}
