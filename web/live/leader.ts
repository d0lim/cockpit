import { Fanout, type Open } from "./fanout";
import { type Live, Router } from "./router";
import { IDLE_MS, PING_MS } from "./serve";
import type { Wire } from "./wire";

export const LEADER_LOCK = "cockpit-live-leader";

export type Locks = { request(name: string, callback: () => Promise<void>): Promise<unknown> };
export type Channel = { postMessage(m: Message): void; onmessage: ((e: MessageEvent) => void) | null; close(): void };

export type Message =
	| { t: "want"; tab: string; targets: string[] }
	| { t: "bye"; tab: string }
	| { t: "hello"; leader: string }
	| { t: "wire"; w: Wire }
	| { t: "to"; tab: string; wires: Wire[] };

type Options = {
	channel: Channel;
	locks: Locks;
	open: Open;
	tab?: string;
	now?: () => number;
	every?: (fn: () => void, ms: number) => () => void;
};

export type LeaderLive = Live & { dispose(): void; leading(): boolean };

const defaultEvery = (fn: () => void, ms: number) => {
	const id = setInterval(fn, ms);
	return () => clearInterval(id);
};

export function leaderLive(o: Options): LeaderLive {
	const tab = o.tab ?? crypto.randomUUID();
	const now = o.now ?? Date.now;
	const tick = o.every ?? defaultEvery;
	let fan: Fanout<string> | null = null;
	let release: (() => void) | null = null;
	let disposed = false;
	const held = new Map<string, Set<string>>();
	const heard = new Map<string, number>();
	const announce = () => {
		const targets = router.targets();
		if (fan !== null) apply(tab, targets);
		else o.channel.postMessage({ t: "want", tab, targets });
	};
	const router: Router = new Router({ first: announce, last: announce });
	const apply = (from: string, targets: string[]) => {
		if (fan === null) return;
		heard.set(from, now());
		const before = held.get(from) ?? new Set<string>();
		const after = new Set(targets);
		held.set(from, after);
		for (const t of after) if (!before.has(t)) fan.subscribe(from, t);
		for (const t of before) if (!after.has(t)) fan.unsubscribe(from, t);
	};
	const forget = (from: string) => {
		held.delete(from);
		heard.delete(from);
		fan?.drop(from);
	};
	const lead = () => {
		fan = new Fanout<string>(o.open, {
			live: (subs, w) => {
				o.channel.postMessage({ t: "wire", w });
				if (subs.has(tab)) router.deliver(w);
			},
			replay: (sub, wires) => {
				if (sub === tab) wires.forEach(router.deliver);
				else if (wires.length > 0) o.channel.postMessage({ t: "to", tab: sub, wires: [...wires] });
			},
		});
		apply(tab, router.targets());
		o.channel.postMessage({ t: "hello", leader: tab });
	};
	o.channel.onmessage = (e) => {
		const m = e.data as Message;
		if (m.t === "want") apply(m.tab, m.targets);
		else if (m.t === "bye") forget(m.tab);
		else if (m.t === "hello" && fan === null) announce();
		else if (m.t === "wire" && fan === null) router.deliver(m.w);
		else if (m.t === "to" && m.tab === tab) m.wires.forEach(router.deliver);
	};
	void o.locks.request(LEADER_LOCK, () => {
		if (disposed) return Promise.resolve();
		lead();
		return new Promise<void>((resolve) => {
			release = resolve;
		});
	});
	const stop = tick(() => {
		if (fan === null) {
			announce();
			return;
		}
		const at = now();
		for (const [from, last] of heard) if (from !== tab && at - last > IDLE_MS) forget(from);
	}, PING_MS);
	return {
		subscribe: (target, push) => router.subscribe(target, push),
		leading: () => fan !== null,
		dispose: () => {
			if (disposed) return;
			disposed = true;
			stop();
			o.channel.postMessage({ t: "bye", tab });
			fan?.close();
			fan = null;
			release?.();
			o.channel.close();
		},
	};
}
