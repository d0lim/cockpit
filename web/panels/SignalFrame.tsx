import type { ReactNode } from "react";
import type { CollectorState, Resync, RunItem } from "../shared/types";
import { EMPTY_SENTENCES, emptyKind, holeNotice } from "../signals/model";
import { Notice, Panel } from "../ui";
import { CollectorStatus } from "./CollectorStatus";

export type SignalMeta = { frozen: boolean; frozenSeq: number; dropped: Record<string, number>; collectors: CollectorState[] };

export const NO_META: SignalMeta = { frozen: false, frozenSeq: 0, dropped: {}, collectors: [] };

type Props = { title: string; role: string; run: RunItem; meta: SignalMeta; holes: readonly Resync[]; refused: string | null; count: number; children: ReactNode };

export function SignalFrame({ title, role, run, meta, holes, refused, count, children }: Props) {
	const empty = emptyKind(run, meta.collectors, count);
	return (
		<Panel title={title}>
			<p className="muted role">{role}</p>
			<CollectorStatus collectors={meta.collectors} />
			{refused ? <Notice>{refused}</Notice> : null}
			{holes.map((h) => (
				<Notice key={`${h.from}-${h.to}`}>{holeNotice(h.from, h.to)}</Notice>
			))}
			{empty ? <Notice>{EMPTY_SENTENCES[empty]}</Notice> : null}
			{count > 0 ? children : null}
		</Panel>
	);
}
