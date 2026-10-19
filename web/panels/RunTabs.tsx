import type { FlowPick, TracePick } from "../nav";
import { isSignalTab, served, type Tab, TAB_LABELS, TAB_ROLES, UNSERVED_NOTICE } from "../shared/tabs";
import type { HubList, RunItem, Signal } from "../shared/types";
import { eventRows } from "../signals/events";
import { LOG_COLLECTOR, latestRevisions } from "../signals/model";
import type { Live } from "../store";
import { Notice } from "../ui";
import { Jobs } from "./Jobs";
import { Logs } from "./Logs";
import { Report } from "./Report";
import { SignalEvents } from "./SignalEvents";
import { SignalFrame, type SignalMeta } from "./SignalFrame";
import { SignalTables } from "./SignalTable";
import { SignalTrace } from "./SignalTrace";

type Props = {
	tab: Tab;
	id: string;
	run: RunItem | undefined;
	hub: HubList | null;
	live: Pick<Live, "jobs" | "signals" | "holes" | "refused">;
	meta: SignalMeta;
	flowPick: FlowPick;
	tracePick: TracePick;
	onFlow: (pick: FlowPick) => void;
	onTrace: (pick: TracePick) => void;
};

const countFor = (tab: Tab, signals: readonly Signal[]): number => {
	if (tab === "trace") return signals.filter((s) => s.kind === "span").length;
	if (tab === "events") return eventRows(signals).length;
	if (tab === "logs") return signals.filter((s) => s.collector === LOG_COLLECTOR).length;
	return signals.filter((s) => s.kind !== "span" && s.attrs.table !== undefined).length;
};

export function RunTab({ tab, id, run, hub, live, meta, flowPick, tracePick, onFlow, onTrace }: Props) {
	if (tab === "output") return <Jobs jobs={live.jobs} empty={jobsEmpty(run)} />;
	if (!run) return <p className="muted">This run is not in the run list</p>;
	if (!served(run, hub)) return <Notice>{UNSERVED_NOTICE(run.hostApi)}</Notice>;
	if (tab === "report") {
		return (
			<>
				<p className="muted role">{TAB_ROLES.report}</p>
				<Report id={id} run={run} pick={flowPick} onPick={onFlow} />
			</>
		);
	}
	if (!isSignalTab(tab)) return null;
	const signals = latestRevisions(live.signals);
	return (
		<SignalFrame title={TAB_LABELS[tab]} role={TAB_ROLES[tab]} run={run} meta={meta} holes={live.holes} refused={live.refused} count={countFor(tab, signals)}>
			{tab === "trace" ? <SignalTrace rows={signals} meta={meta} pick={tracePick} onPick={onTrace} /> : null}
			{tab === "events" ? <SignalEvents rows={eventRows(signals)} meta={meta} /> : null}
			{tab === "table" ? <SignalTables rows={signals} meta={meta} /> : null}
			{tab === "logs" ? <Logs signals={signals} /> : null}
		</SignalFrame>
	);
}

function jobsEmpty(run: RunItem | undefined): string {
	if (run?.phase === "queued") return "Stage summaries stream here once the run starts.";
	return "This hub holds no job output for the run. The report shows the last lines of the stage that stopped.";
}

export function TabBar({ tabs, tab, onTab }: { tabs: readonly Tab[]; tab: Tab; onTab: (t: Tab) => void }) {
	return (
		<nav className="tabs" aria-label="Run tabs">
			{tabs.map((t) => (
				<button type="button" key={t} className={t === tab ? "active" : undefined} aria-pressed={t === tab} onClick={() => onTab(t)}>
					{TAB_LABELS[t]}
				</button>
			))}
		</nav>
	);
}
