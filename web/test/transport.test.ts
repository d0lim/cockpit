import { afterEach, describe, expect, test } from "bun:test";
import type { Locks } from "../live/leader";
import type { PortLike } from "../live/serve";
import { chooseLive, type Env } from "../live/transport";
import { WORKER_NAME, WORKER_URL } from "../live/wire";
import { FakeSource } from "./fakeSource";

const hides: (() => void)[] = [];
afterEach(() => {
	for (const h of hides.splice(0)) h();
});

const sources: FakeSource[] = [];
const base = (): Env => ({ source: (url) => (sources.push(new FakeSource(url)), sources.at(-1) as FakeSource), onHide: (fn) => hides.push(fn) });
const locks: Locks = { request: () => new Promise(() => {}) };

describe("choosing how a tab reaches the event stream", () => {
	test("a shared worker is used when the browser has one, under the protocol name", () => {
		const made: { url: string; name: string; posted: unknown[] }[] = [];
		class Worker {
			port: PortLike;
			constructor(url: string, options: { name: string }) {
				const posted: unknown[] = [];
				made.push({ url, name: options.name, posted });
				this.port = { postMessage: (m) => posted.push(m), onmessage: null };
			}
		}
		const chosen = chooseLive({ ...base(), SharedWorker: Worker, BroadcastChannel, locks });
		chosen.live.subscribe("hub", () => {});
		expect([chosen.kind, made.length, made[0]?.url, made[0]?.name]).toEqual(["worker", 1, WORKER_URL, WORKER_NAME]);
		expect(made[0]?.posted).toEqual([{ kind: "sub", target: "hub" }]);
		expect([sources.length, hides.length]).toEqual([0, 1]);
	});

	test("a worker that cannot start falls back to the leader channel, and without channel or locks to one stream per tab", () => {
		class Broken {
			port: PortLike = { postMessage: () => {}, onmessage: null };
			constructor() {
				throw new Error("blocked");
			}
		}
		const leader = chooseLive({ ...base(), SharedWorker: Broken, BroadcastChannel, locks });
		const noLocks = chooseLive({ ...base(), BroadcastChannel });
		const bare = chooseLive(base());
		expect([leader.kind, noLocks.kind, bare.kind]).toEqual(["leader", "local", "local"]);
		const before = sources.length;
		bare.live.subscribe("hub", () => {});
		bare.live.subscribe("hub", () => {});
		expect(sources.length - before).toBe(1);
	});
});
