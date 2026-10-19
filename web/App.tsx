import { useEffect, useState } from "react";
import { fetchSignalMeta, useLive, usePoll } from "./api";
import { type FlowPick, type Nav, readNav, type TracePick, writeNav } from "./nav";
import { RunSide } from "./panels/RunDetail";
import { Header } from "./panels/Header";
import { RunTab, TabBar } from "./panels/RunTabs";
import { Home, Runs } from "./panels/Runs";
import { NO_META, type SignalMeta } from "./panels/SignalFrame";
import { defaultTab, type Tab, tabsFor } from "./shared/tabs";
import { latestRevisions } from "./signals/model";

const META_MS = 2000;

export function App() {
	const [nav, setNav] = useState<Nav>(readNav);
	const live = useLive(nav.run);
	const run = nav.run === null ? undefined : live.hub?.runs.find((r) => r.id === nav.run);
	const tabs = tabsFor(run, live.hub);
	const tab: Tab = nav.tab !== null && tabs.includes(nav.tab) ? nav.tab : run ? defaultTab(run, live.hub) : "output";
	const meta = usePoll(() => (nav.run === null ? Promise.resolve(null) : fetchSignalMeta(nav.run, 0)), nav.run === null ? null : META_MS, [nav.run]);
	const signalMeta: SignalMeta = meta.data ? { frozen: meta.data.frozen, frozenSeq: meta.data.frozenSeq, dropped: meta.data.dropped, collectors: meta.data.collectors } : NO_META;

	useEffect(() => {
		writeNav(nav);
	}, [nav]);
	useEffect(() => {
		if (nav.tab === null && run) setNav((cur) => ({ ...cur, tab: defaultTab(run, live.hub) }));
	}, [nav.tab, run, live.hub]);

	const select = (id: string | null) => setNav((cur) => (cur.run === id ? cur : { run: id, tab: null, flow: null, step: null, trace: null, span: null }));
	const setTab = (t: Tab) => setNav((cur) => ({ ...cur, tab: t }));
	const pickFlow = (pick: FlowPick) => setNav((cur) => ({ ...cur, ...pick }));
	const pickTrace = (pick: TracePick) => setNav((cur) => ({ ...cur, ...pick }));

	return (
		<div className="app">
			<Header run={nav.run} item={run} hub={live.hub} hubAt={live.hubAt} connection={live.connection} onSelect={select} />
			<div className="layout">
				<aside>
					<Runs hub={live.hub} selected={nav.run} onSelect={select} />
					{nav.run === null ? null : <RunSide key={nav.run} id={nav.run} run={run} hub={live.hub} signals={latestRevisions(live.signals)} onSelect={select} />}
				</aside>
				<main key={nav.run ?? "home"}>
					{nav.run === null ? (
						<Home hub={live.hub} />
					) : (
						<>
							<TabBar tabs={tabs} tab={tab} onTab={setTab} />
							<RunTab
								tab={tab}
								id={nav.run}
								run={run}
								hub={live.hub}
								live={live}
								meta={signalMeta}
								flowPick={nav}
								tracePick={nav}
								onFlow={pickFlow}
								onTrace={pickTrace}
							/>
						</>
					)}
				</main>
			</div>
		</div>
	);
}
