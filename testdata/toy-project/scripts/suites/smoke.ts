import { stage, suite } from "cockpit:flow";
import { facts } from "../flows/facts";
import { health } from "../flows/health";
import { orders } from "../flows/orders";

export default suite({
	name: "smoke",
	stages: [stage({ id: "basic", title: "Basic" }), stage({ id: "observe", title: "Observe" })],
	flows: [health, orders, facts],
});
