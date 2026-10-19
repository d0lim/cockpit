import { beforeEach, describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { SignalEvents } from "../panels/SignalEvents";
import { SignalFrame, type SignalMeta } from "../panels/SignalFrame";
import { SignalTable, SignalTables } from "../panels/SignalTable";
import { SignalTrace } from "../panels/SignalTrace";
import { EMPTY_SENTENCES, FROZEN_DIVIDER } from "../signals/model";
import { tables } from "../signals/table";
import { event, RUN, resetSeq, run, span } from "./fixtures";

beforeEach(resetSeq);

const META: SignalMeta = { frozen: true, frozenSeq: 50, dropped: { "tail-cap": 7 }, collectors: [{ id: "toy-db", state: "armed", failures: 0 }] };
const count = (html: string, needle: string) => html.split(needle).length - 1;

describe("the events view renders", () => {
	test("status groups with their counts, a neutral badge for statuses without tone, the divider and the dropped line", () => {
		const rows = Array.from({ length: 100 }, (_, i) => (i % 2 === 0 ? event({ status: "sent", tone: "ok", label: `n${i}` }) : event({ status: "queued", label: `n${i}` })));
		const html = renderToStaticMarkup(<SignalEvents rows={rows} meta={META} />);
		expect(html).toContain('data-status="sent"><span class="badge ok">sent</span> 50');
		expect(html).toContain('data-status="queued"><span class="badge neutral">queued</span> 50');
		expect(count(html, FROZEN_DIVIDER)).toBe(1);
		expect(html).toContain("7 older records dropped (tail-cap 7)");
		expect(html.indexOf(FROZEN_DIVIDER)).toBeLessThan(html.indexOf("7 older records dropped"));
		expect(count(html, ">in bundle<")).toBe(50);
		expect(count(html, ">after the bundle<")).toBe(50);
		expect(html).toContain("Filters apply to the latest 100 of 100 events read.");
	});

	test("the picked event opens its attributes and marks a long body as truncated", () => {
		const long = event({ status: "sent", label: "big" }, { id: "n-9", event: { body: "x".repeat(5000) } });
		const html = renderToStaticMarkup(<SignalEvents rows={[event({ status: "sent" }), long]} meta={{ ...META, frozen: false }} initialSelected="toy-db/n-9" />);
		expect(html).toContain('aria-label="Event detail"');
		expect(html).toContain("<dt>label</dt><dd>big</dd>");
		expect(html).toContain("The body is truncated on screen.");
		const plain = renderToStaticMarkup(<SignalEvents rows={[event({ status: "sent" })]} meta={META} />);
		expect(plain).not.toContain('aria-label="Event detail"');
		expect(plain).toContain("Filters apply");
	});
});

describe("the table view renders", () => {
	test("one section per table value with the union of attributes as columns, and hidden columns leave the header", () => {
		const rows = [event({ table: "jobs", label: "a", owner: "x" }), event({ table: "queues", label: "q", depth: 3 })];
		const html = renderToStaticMarkup(<SignalTables rows={rows} meta={META} />);
		expect(html).toContain('aria-label="table jobs"');
		expect(html).toContain('aria-label="table queues"');
		const jobs = tables(rows)[0];
		if (!jobs) throw new Error("no jobs table");
		const hidden = renderToStaticMarkup(<SignalTable data={jobs} meta={META} initialHidden={["owner"]} />);
		expect(hidden).toContain("<th>label</th>");
		expect(hidden).not.toContain("<th>owner</th>");
		expect(hidden).toContain('aria-pressed="false">owner</button>');
	});
});

describe("the trace view renders", () => {
	test("spans in parent and child order with incomplete and no-end text, and the picked span offers its links", () => {
		const rows = [
			span("step:a", "flow:f", 20, 50),
			span("run", undefined, 0, undefined, {}, "incomplete"),
			span("flow:f", "run", 10, 100, { span: { traceId: RUN, parentId: "run", name: "flow:f", start: "2026-10-19T01:00:00.010Z", end: "2026-10-19T01:00:00.100Z", status: "ok", links: [{ traceId: `${RUN}/exec/one`, spanId: "exec" }] } }),
		];
		const html = renderToStaticMarkup(<SignalTrace rows={rows} meta={{ ...META, frozen: false }} pick={{ trace: RUN, span: "flow:f" }} onPick={() => undefined} />);
		const order = ["run", "flow:f", "step:a"].map((name) => html.indexOf(`>${name}</span>`));
		expect(order).toEqual([...order].sort((a, b) => a - b));
		expect(order.every((i) => i > 0)).toBe(true);
		expect(html).toContain("no end · incomplete");
		expect(html).toContain('class="span-bar marker"');
		expect(html).toContain(`Follow the link to ${RUN}/exec/one`);
		expect(html).toContain('aria-label="Span detail"');
	});
});

describe("the signal frame renders", () => {
	test("a collector row shows the backoff state, failure count and reason", () => {
		const meta = { ...META, collectors: [{ id: "toy-db", state: "backoff", failures: 4, reason: "error: boom", lastOkAt: "2026-10-19T01:00:00Z" }] };
		const html = renderToStaticMarkup(
			<SignalFrame title="Events" role="r" run={run({ phase: "done" })} meta={meta} holes={[{ from: 3, to: 9 }]} refused={null} count={1}>
				<p>body</p>
			</SignalFrame>,
		);
		expect(html).toContain("Backing off");
		expect(html).toContain("toy-db · Backing off · last success");
		expect(html).toContain("failures 4 · reason error: boom");
		expect(html).toContain("Signals 3-9 were dropped before this screen read them");
		expect(html).toContain("<p>body</p>");
	});

	test("the four empty states render four different sentences", () => {
		const armed = [{ id: "toy-db", state: "armed", failures: 0 }];
		const cases = [
			{ r: run({ phase: "sealing", env: "absent" }), c: armed, s: EMPTY_SENTENCES["before-boot"] },
			{ r: run({ phase: "done", env: "stopped" }), c: armed, s: EMPTY_SENTENCES["env-off"] },
			{ r: run({ phase: "done", env: "up" }), c: [], s: EMPTY_SENTENCES["no-collectors"] },
			{ r: run({ phase: "done", env: "up" }), c: armed, s: EMPTY_SENTENCES["no-signals"] },
		];
		const seen = new Set<string>();
		for (const k of cases) {
			const html = renderToStaticMarkup(
				<SignalFrame title="Trace" role="r" run={k.r} meta={{ ...META, collectors: k.c }} holes={[]} refused={null} count={0}>
					<p>hidden</p>
				</SignalFrame>,
			);
			expect(html).toContain(k.s);
			expect(html).not.toContain("<p>hidden</p>");
			seen.add(k.s);
		}
		expect(seen.size).toBe(4);
	});
});
