import { isPending, stoppable } from "./pool";
import type { HubList, RunItem } from "./types";

export const RUN_TABS = ["output", "trace", "events", "table", "logs", "report"] as const;
export type Tab = (typeof RUN_TABS)[number];

export const TAB_LABELS: Readonly<Record<Tab, string>> = {
	output: "Job output",
	trace: "Trace",
	events: "Events",
	table: "Table",
	logs: "Logs",
	report: "Report",
};

export const TAB_ROLES: Readonly<Record<Tab, string>> = {
	output: "The pipeline stage summary lines of this run.",
	trace: "Run, flow, step and exec spans as a timeline.",
	events: "Read what happened in time order.",
	table: "Compare attributes as columns.",
	logs: "Container log lines, one source at a time.",
	report: "The verdict, flow results and the report bundle.",
};

const SIGNAL_TABS: ReadonlySet<Tab> = new Set<Tab>(["trace", "events", "table", "logs"]);

export const isTab = (value: string | null): value is Tab => value !== null && (RUN_TABS as readonly string[]).includes(value);

export const isSignalTab = (tab: Tab): boolean => SIGNAL_TABS.has(tab);

export function served(run: Pick<RunItem, "hostApi">, hub: Pick<HubList, "servedHostApis"> | null): boolean {
	return hub === null || hub.servedHostApis.includes(run.hostApi);
}

export const UNSERVED_NOTICE = (api: number) =>
	`This hub does not serve host API ${api}. Signals and the report are hidden; the run can only be listed and destroyed.`;

export function tabsFor(run: RunItem | undefined, hub: Pick<HubList, "servedHostApis"> | null): readonly Tab[] {
	if (run === undefined || served(run, hub)) return RUN_TABS;
	return RUN_TABS.filter((t) => t === "output");
}

export function defaultTab(run: RunItem, hub: Pick<HubList, "servedHostApis"> | null): Tab {
	if (isPending(run) || !served(run, hub)) return "output";
	return stoppable(run) ? "trace" : "report";
}
