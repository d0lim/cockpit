import { describe, expect, test } from "bun:test";
import { EMPTY_NAV, navHash, parseNav } from "../nav";
import { defaultTab, RUN_TABS, type Tab, tabsFor } from "../shared/tabs";
import type { Env, Phase } from "../shared/types";
import { hub, RUN, run } from "./fixtures";

describe("run tabs", () => {
	test("the default tab is job output while queued or active, trace for an up kept run and report once stopped or closed", () => {
		const cases: [Phase, Env, Tab][] = [
			["queued", "absent", "output"],
			["building", "absent", "output"],
			["testing", "up", "output"],
			["reporting", "up", "output"],
			["done", "up", "trace"],
			["done", "stopped", "report"],
			["done", "absent", "report"],
			["done", "destroyed", "report"],
		];
		const got = cases.map(([phase, env]) => defaultTab(run({ phase, env }), hub([])));
		expect(got).toEqual(cases.map((c) => c[2]));
		expect(cases.length).toBe(8);
	});

	test("a run on a host API the hub does not serve shows only job output and defaults to it", () => {
		const list = hub([], { servedHostApis: [2] });
		const old = run({ phase: "done", env: "up", hostApi: 1 });
		expect(tabsFor(old, list)).toEqual(["output"]);
		expect(defaultTab(old, list)).toBe("output");
		expect(tabsFor(run({ hostApi: 2 }), list)).toEqual(RUN_TABS);
		expect(tabsFor(old, hub([]))).toEqual(RUN_TABS);
	});

	test("the address hash restores the run, the tab and the picked trace and span", () => {
		const nav = parseNav(`#run=${RUN}&tab=trace&trace=${encodeURIComponent(`${RUN}/exec/again`)}&span=flow%3Aboot`);
		expect(nav).toEqual({ run: RUN, tab: "trace", flow: null, step: null, trace: `${RUN}/exec/again`, span: "flow:boot" });
		expect(parseNav(navHash(nav))).toEqual(nav);
		const report = parseNav(`#run=${RUN}&tab=report&flow=transfer-allow&step=s1`);
		expect([report.flow, report.step]).toEqual(["transfer-allow", "s1"]);
	});

	test("hash values that do not belong to the tab or are malformed are dropped", () => {
		expect(parseNav(`#run=${RUN}&tab=events&trace=x&flow=y`)).toEqual({ ...EMPTY_NAV, run: RUN, tab: "events" });
		expect(parseNav(`#run=${RUN}&tab=report&flow=Bad_Flow&step=s1`)).toEqual({ ...EMPTY_NAV, run: RUN, tab: "report" });
		expect(parseNav("#run=../etc&tab=trace")).toEqual(EMPTY_NAV);
		expect(parseNav(`#run=${RUN}&tab=flows`)).toEqual({ ...EMPTY_NAV, run: RUN });
		expect(navHash(EMPTY_NAV)).toBe("");
	});
});
