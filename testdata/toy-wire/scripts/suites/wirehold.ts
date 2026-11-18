import { stage, suite } from "cockpit:flow";
import { hold } from "../flows/wire-calls";

export default suite({
	name: "wirehold",
	stages: [stage({ id: "calls", title: "Calls" })],
	flows: [hold],
});
