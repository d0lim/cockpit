import { Fanout, type Open } from "./fanout";
import { type Locks, leaderLive } from "./leader";
import { type Live, Router } from "./router";
import { PING_MS, type PortLike } from "./serve";
import { openSources, type Source } from "./source";
import { type Replay, WORKER_NAME, WORKER_URL, type Wire } from "./wire";

export type Every = (fn: () => void, ms: number) => () => void;

export const every: Every = (fn, ms) => {
	const id = setInterval(fn, ms);
	return () => clearInterval(id);
};

export function workerLive(port: PortLike, tick: Every = every): Live & { close(): void } {
	const router = new Router({
		first: (target) => port.postMessage({ kind: "sub", target }),
		last: (target) => port.postMessage({ kind: "unsub", target }),
	});
	port.onmessage = (e) => {
		const m = e.data as Wire | Replay;
		if (m.kind === "replay") m.wires.forEach(router.deliver);
		else router.deliver(m);
	};
	port.start?.();
	const stop = tick(() => port.postMessage({ kind: "ping" }), PING_MS);
	return {
		subscribe: (target, push) => router.subscribe(target, push),
		close: () => {
			stop();
			port.postMessage({ kind: "bye" });
		},
	};
}

export function localLive(open: Open): Live {
	const fan: Fanout<Router> = new Fanout<Router>(open, {
		live: (subs, w) => {
			for (const r of subs) r.deliver(w);
		},
		replay: (r, wires) => wires.forEach(r.deliver),
	});
	const router: Router = new Router({ first: (target) => fan.subscribe(router, target), last: (target) => fan.unsubscribe(router, target) });
	return router;
}

type WorkerCtor = new (url: string, options: { type: "module"; name: string }) => { port: PortLike };
type ChannelCtor = new (name: string) => BroadcastChannel;

export type Env = { SharedWorker?: WorkerCtor; BroadcastChannel?: ChannelCtor; locks?: Locks; source: (url: string) => Source; onHide?: (fn: () => void) => void };

export type Chosen = { kind: "worker" | "leader" | "local"; live: Live };

export function chooseLive(env: Env): Chosen {
	const open = openSources(env.source);
	if (env.SharedWorker !== undefined) {
		try {
			const live = workerLive(new env.SharedWorker(WORKER_URL, { type: "module", name: WORKER_NAME }).port);
			env.onHide?.(live.close);
			return { kind: "worker", live };
		} catch {
			return choosePage(env, open);
		}
	}
	return choosePage(env, open);
}

function choosePage(env: Env, open: Open): Chosen {
	if (env.BroadcastChannel !== undefined && env.locks !== undefined) {
		const live = leaderLive({ channel: new env.BroadcastChannel(WORKER_NAME), locks: env.locks, open });
		env.onHide?.(live.dispose);
		return { kind: "leader", live };
	}
	return { kind: "local", live: localLive(open) };
}
