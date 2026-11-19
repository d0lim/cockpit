import { stage, suite } from "cockpit:flow";
import { credentials, learned, recall } from "../flows/wire-values";
import { bodies, callback, closing, relay, timing, up } from "../flows/wire-calls";

export default suite({
	name: "wire",
	stages: [stage({ id: "calls", title: "Calls" }), stage({ id: "values", title: "Values" }), stage({ id: "bodies", title: "Bodies" }), stage({ id: "end", title: "End" })],
	flows: [up, relay, timing, callback, credentials, learned, recall, bodies, closing],
});
