import { IDLE_MS, PING_MS, WorkerHub } from "./serve";
import { openSources } from "./source";

const hub = new WorkerHub(openSources((url) => new EventSource(url)));
const scope = globalThis as unknown as { onconnect: ((e: MessageEvent) => void) | null };
scope.onconnect = (e) => {
	const port = e.ports[0];
	if (port !== undefined) hub.connect(port);
};
setInterval(() => hub.sweep(IDLE_MS), PING_MS);
