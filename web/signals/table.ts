import type { Signal } from "../shared/types";
import { attrText } from "./model";

export type SignalTableData = { name: string; columns: string[]; rows: Signal[] };

const FIRST = ["label", "status", "tone"];

export function tables(signals: readonly Signal[]): SignalTableData[] {
	const byName = new Map<string, Signal[]>();
	for (const s of signals) {
		if (s.kind === "span") continue;
		const name = attrText(s.attrs.table);
		if (name === "") continue;
		byName.set(name, [...(byName.get(name) ?? []), s]);
	}
	return [...byName.entries()].map(([name, rows]) => ({ name, rows, columns: columnsOf(rows) }));
}

export function columnsOf(rows: readonly Signal[]): string[] {
	const keys = new Set<string>();
	for (const s of rows) {
		for (const key of Object.keys(s.attrs)) if (key !== "table") keys.add(key);
		if (s.metric) keys.add("value");
	}
	const first = FIRST.filter((k) => keys.has(k));
	return [...first, ...[...keys].filter((k) => !FIRST.includes(k)).sort()];
}

export const cellOf = (s: Signal, column: string): string => (column === "value" && s.metric ? String(s.metric.value) : attrText(s.attrs[column]));
