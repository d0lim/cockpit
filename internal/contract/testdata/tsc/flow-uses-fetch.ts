import { flow, step } from "cockpit:flow";

export default flow({
	id: "leaky",
	title: "러너 자리에서 fetch 를 부른다",
	stage: "basic",
	once: false,
	requires: [],
	steps: [
		step({
			id: "call",
			title: "밖으로 나간다",
			kind: "request",
			async run() {
				await fetch("http://example.com");
			},
		}),
	],
});
