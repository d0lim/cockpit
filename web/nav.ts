import { isTab, type Tab } from "./shared/tabs";

const RUN_ID = /^run-[0-9]{8}-[0-9]{6}-[0-9a-f]{7}(-[0-9]+)?$/;
export const NAV_FLOW_ID = /^[a-z0-9][a-z0-9-]{0,62}$/;
const MAX_ID = 200;

export type FlowPick = { flow: string | null; step: string | null };
export type TracePick = { trace: string | null; span: string | null };
export type Nav = { run: string | null; tab: Tab | null } & FlowPick & TracePick;

export const EMPTY_NAV: Nav = { run: null, tab: null, flow: null, step: null, trace: null, span: null };

export const isRunId = (value: string | null): value is string => value !== null && value.length <= 63 && RUN_ID.test(value);

const shortText = (value: string | null): string | null => (value !== null && value !== "" && value.length <= MAX_ID ? value : null);

function normalize(nav: Nav): Nav {
	if (nav.run === null) return EMPTY_NAV;
	const flow = nav.tab === "report" && nav.flow !== null && NAV_FLOW_ID.test(nav.flow) ? nav.flow : null;
	const trace = nav.tab === "trace" ? shortText(nav.trace) : null;
	return {
		run: nav.run,
		tab: nav.tab,
		flow,
		step: flow === null ? null : shortText(nav.step),
		trace,
		span: trace === null ? null : shortText(nav.span),
	};
}

export function parseNav(hash: string): Nav {
	const params = new URLSearchParams(hash.replace(/^#/, ""));
	const run = params.get("run");
	const tab = params.get("tab");
	return normalize({
		run: isRunId(run) ? run : null,
		tab: isTab(tab) ? tab : null,
		flow: params.get("flow"),
		step: params.get("step"),
		trace: params.get("trace"),
		span: params.get("span"),
	});
}

export function navHash(nav: Nav): string {
	const n = normalize(nav);
	if (n.run === null) return "";
	const params = new URLSearchParams({ run: n.run });
	for (const key of ["tab", "flow", "step", "trace", "span"] as const) {
		const value = n[key];
		if (value !== null) params.set(key, value);
	}
	return `#${params}`;
}

export const readNav = (): Nav => parseNav(location.hash);

export function writeNav(nav: Nav): void {
	const hash = navHash(nav);
	if (location.hash !== hash) history.replaceState(null, "", hash === "" ? location.pathname : hash);
}
