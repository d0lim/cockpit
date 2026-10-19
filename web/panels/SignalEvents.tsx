import { Fragment, useState } from "react";
import { toneOf } from "../shared/classify";
import type { Signal } from "../shared/types";
import { bodyText, type Chip, chipsOf, groups, matches, recent, scopeLine, selectionKey, statusOf } from "../signals/events";
import { attrText, PLACEMENT_LABELS, placement, withBoundaries } from "../signals/model";
import { Badge, time } from "../ui";
import { Divider, Dropped } from "./Boundary";
import type { SignalMeta } from "./SignalFrame";

type Props = { rows: readonly Signal[]; meta: SignalMeta; initialChips?: Chip[]; initialSelected?: string | null };

const chipKey = (c: Chip) => `${c.key}=${c.value}`;

export function SignalEvents({ rows, meta, initialChips = [], initialSelected = null }: Props) {
	const [chips, setChips] = useState<Chip[]>(initialChips);
	const [selected, setSelected] = useState<string | null>(initialSelected);
	const latest = recent(rows);
	const shown = latest.filter((s) => matches(s, chips));
	const picked = shown.find((s) => selectionKey(s) === selected) ?? null;
	const toggle = (c: Chip) => setChips((cur) => (cur.some((x) => chipKey(x) === chipKey(c)) ? cur.filter((x) => chipKey(x) !== chipKey(c)) : [...cur, c]));
	return (
		<div className="signal-view">
			<div className="groups" aria-label="Status groups">
				{groups(shown).map((g) => (
					<span key={g.status} className="group" data-status={g.status}>
						<Badge tone={g.tone}>{g.status}</Badge> {g.count}
					</span>
				))}
			</div>
			<div className="chips" aria-label="Filters">
				{chipsOf(latest).map((c) => {
					const on = chips.some((x) => chipKey(x) === chipKey(c));
					return (
						<button key={chipKey(c)} type="button" className={on ? "chip on" : "chip"} aria-pressed={on} onClick={() => toggle(c)}>
							{c.key}: {c.value}
						</button>
					);
				})}
			</div>
			<p className="muted scope">{scopeLine(latest.length, rows.length)}</p>
			<div className="split">
				<ol className="events">
					{withBoundaries(shown, meta).map((item, i) => {
						if (item.kind === "divider") return <Divider key={`divider-${i}`} />;
						if (item.kind === "dropped") return <Dropped key={`dropped-${i}`} text={item.text} />;
						const s = item.row;
						const key = selectionKey(s);
						const where = PLACEMENT_LABELS[placement(s.seq, meta.frozen, meta.frozenSeq)];
						return (
							<li key={`${key}-${s.seq}`}>
								<button type="button" className={key === selected ? "event selected" : "event"} aria-pressed={key === selected} onClick={() => setSelected(key)}>
									<span className="muted">{time(s.ts)}</span>
									<Badge tone={toneOf(s.attrs.tone)}>{statusOf(s)}</Badge>
									<span>{attrText(s.attrs.label) || s.event?.body || s.collector}</span>
									<span className="muted">{s.collector}</span>
									{s.rev ? <span className="muted">rev {s.rev}</span> : null}
									{where ? <span className="muted">{where}</span> : null}
								</button>
							</li>
						);
					})}
				</ol>
				{picked ? <EventDetail signal={picked} /> : null}
			</div>
		</div>
	);
}

export function EventDetail({ signal }: { signal: Signal }) {
	const body = bodyText(signal);
	return (
		<aside className="detail" aria-label="Event detail">
			<dl className="kv">
				<dt>seq</dt>
				<dd>{signal.seq}</dd>
				<dt>collector</dt>
				<dd>{signal.collector}</dd>
				{signal.id ? (
					<>
						<dt>id</dt>
						<dd>
							{signal.id}
							{signal.rev ? ` · rev ${signal.rev}` : ""}
						</dd>
					</>
				) : null}
				<dt>time</dt>
				<dd>
					{signal.ts} · {signal.tsSource}
				</dd>
				{Object.entries(signal.attrs).map(([k, v]) => (
					<Fragment key={k}>
						<dt>{k}</dt>
						<dd>{String(v)}</dd>
					</Fragment>
				))}
			</dl>
			<pre className="console">{body.text}</pre>
			{body.truncated ? <p className="needs">The body is truncated on screen.</p> : null}
		</aside>
	);
}
