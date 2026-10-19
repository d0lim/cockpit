import { beforeEach, describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { FlowDetail } from "../panels/FlowDetail";
import { Header } from "../panels/Header";
import { Logs } from "../panels/Logs";
import { RunTab, TabBar } from "../panels/RunTabs";
import { Home, Runs } from "../panels/Runs";
import { addressOf } from "../panels/Services";
import { NO_META } from "../panels/SignalFrame";
import { tabsFor } from "../shared/tabs";
import type { FlowView } from "../shared/types";
import { event, hub, RUN, resetSeq, run } from "./fixtures";

beforeEach(resetSeq);

const NO_LIVE = { jobs: [], signals: [], holes: [], refused: null };
const noop = () => undefined;

describe("the kept panels", () => {
	test("the header is titled cockpit and carries no badge or extra target picker while connected", () => {
		const html = renderToStaticMarkup(<Header run={null} item={undefined} hub={hub([run()])} hubAt={null} connection="open" onSelect={noop} />);
		expect(html).toContain("<h1>cockpit</h1>");
		expect(html.split('class="badge').length - 1).toBe(0);
		expect(html.split("<button").length - 1).toBe(1);
		const lost = renderToStaticMarkup(<Header run={RUN} item={run()} hub={hub([run()])} hubAt={null} connection="lost" onSelect={noop} />);
		expect(lost).toContain("disconnected · last update none");
	});

	test("an empty run list and the home body both point to cockpit runs submit and show pool usage", () => {
		const list = hub([], { pool: { size: 2, max: 5 }, up: { count: 1, cap: 3 } });
		const runs = renderToStaticMarkup(<Runs hub={list} selected={null} onSelect={noop} />);
		const home = renderToStaticMarkup(<Home hub={list} />);
		for (const html of [runs, home]) {
			expect(html).toContain("cockpit runs submit");
			expect(html).toContain("kept 2/5 · up 1/3");
		}
	});

	test("the port map opens each offset the way the declaration says", () => {
		const base = { name: "web", service: "web", port: 80, hostPort: 22001, label: "web" };
		expect(addressOf(RUN, { ...base, open: "browser" })).toEqual({ href: `http://${RUN}.localhost:22001`, text: `${RUN}.localhost:22001` });
		expect(addressOf(RUN, { ...base, open: "link" })).toEqual({ href: "http://127.0.0.1:22001", text: "127.0.0.1:22001" });
		expect(addressOf(RUN, { ...base, open: "copy" })).toEqual({ href: null, text: "127.0.0.1:22001" });
		expect(addressOf(RUN, base).href).toBe("http://127.0.0.1:22001");
	});

	test("the log view starts on the first source", () => {
		const signals = [event({ container: "api" }, { collector: "logs", event: { body: "api up" } }), event({ container: "db" }, { collector: "logs", event: { body: "db up" } })];
		const html = renderToStaticMarkup(<Logs signals={signals} />);
		expect(html).toContain("api up");
		expect(html).not.toContain("db up");
		expect(renderToStaticMarkup(<Logs signals={signals} picked="db" />)).toContain("db up");
	});

	test("the flow detail lists steps, marks the picked one and says when a step has no line", () => {
		const view: FlowView = {
			source: "bundle",
			flow: {
				id: "transfer-allow",
				stage: "core",
				title: "Transfer is allowed",
				once: false,
				requires: [],
				source: { file: "flows/transfer.ts", line: 12 },
				status: "failed",
				steps: 2,
				passed: 1,
				failed: 1,
				skipped: 0,
				stepList: [
					{ id: "s1", title: "Send", kind: "request", status: "passed", requests: 1, source: { file: "flows/transfer.ts", line: 20 } },
					{ id: "s2", title: "Check", kind: "wait", status: "failed", requests: 0, message: "timed out" },
				],
			},
		};
		const html = renderToStaticMarkup(<FlowDetail view={view} active="s2" onStep={noop} />);
		expect(html).toContain("line 20");
		expect(html).toContain("no line");
		expect(html).toContain('class="step-row active" aria-pressed="true"');
		expect(html).toContain("timed out");
	});
});

describe("run tabs render", () => {
	test("an unserved host API hides the signal and report tabs and explains why", () => {
		const list = hub([], { servedHostApis: [2] });
		const old = run({ phase: "done", env: "up", hostApi: 1 });
		const bar = renderToStaticMarkup(<TabBar tabs={tabsFor(old, list)} tab="output" onTab={noop} />);
		expect(bar).toContain("Job output");
		for (const hidden of ["Trace", "Events", "Table", "Logs", "Report"]) expect(bar).not.toContain(`>${hidden}<`);
		const body = renderToStaticMarkup(
			<RunTab tab="trace" id={RUN} run={old} hub={list} live={NO_LIVE} meta={NO_META} flowPick={{ flow: null, step: null }} tracePick={{ trace: null, span: null }} onFlow={noop} onTrace={noop} />,
		);
		expect(body).toContain("This hub does not serve host API 1");
	});

	test("each signal tab states its role in one line", () => {
		const up = run({ phase: "done", env: "up" });
		const roles: Record<string, string> = { events: "Read what happened in time order.", table: "Compare attributes as columns." };
		for (const [tab, role] of Object.entries(roles)) {
			const html = renderToStaticMarkup(
				<RunTab tab={tab as "events" | "table"} id={RUN} run={up} hub={hub([up])} live={NO_LIVE} meta={NO_META} flowPick={{ flow: null, step: null }} tracePick={{ trace: null, span: null }} onFlow={noop} onTrace={noop} />,
			);
			expect(html).toContain(role);
		}
	});
});
