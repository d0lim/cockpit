import { describe, expect, test } from "bun:test";
import { label, LABELS, stageLabel } from "../shared/labels";
import { statusLabel } from "../shared/flowTable";
import type { FlowOutcome, StepStatus } from "../shared/types";

describe("labels come from the generated Go table", () => {
	test("every flow outcome and step status has a label of its own", () => {
		const keys: (FlowOutcome | StepStatus | "notProven")[] = ["passed", "failed", "skipped", "notRun", "incomplete", "running", "notProven"];
		const seen = keys.map((k) => statusLabel(k));
		expect(seen).toEqual(["Passed", "Failed", "Skipped", "Not run", "Incomplete", "Running", "Not proven"]);
		expect(seen.length).toBe(7);
	});

	test("collector states and stages read the generated words and unknown keys fall back to the key", () => {
		expect(label("collectors", "backoff")).toBe("Backing off");
		expect(label("collectors", "report-only")).toBe("Report only");
		expect(label("collectors", "paused")).toBe("paused");
		expect(stageLabel("sealing")).toBe("seal check");
		expect(stageLabel(undefined)).toBe("-");
		expect(Object.keys(LABELS).sort()).toEqual(["collectors", "destroy", "flowStatus", "messages", "stages", "states", "stop", "verdicts", "wait"]);
	});
});
