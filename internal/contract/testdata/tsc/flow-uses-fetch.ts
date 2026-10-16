import { flow, step } from "cockpit:flow";

export default flow({
	id: "leaky",
	title: "Calls fetch in the runner",
	stage: "basic",
	once: false,
	requires: [],
	steps: [
		step({
			id: "call",
			title: "Reaches outside",
			kind: "request",
			async run() {
				await fetch("http://example.com");
			},
		}),
	],
});
