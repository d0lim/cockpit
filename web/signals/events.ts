import { NEUTRAL, toneOf } from "../shared/classify";
import type { Signal, Tone } from "../shared/types";
import { attrText, LOG_COLLECTOR } from "./model";

export const EVENT_WINDOW = 500;
export const NO_STATUS = "(no status)";

export type Chip = { key: string; value: string };
export type Group = { status: string; count: number; tone: Tone | typeof NEUTRAL };

export const eventRows = (signals: readonly Signal[]): Signal[] => signals.filter((s) => s.kind === "event" && s.collector !== LOG_COLLECTOR);

export const recent = (rows: readonly Signal[], n = EVENT_WINDOW): Signal[] => rows.slice(Math.max(0, rows.length - n));

export const statusOf = (s: Signal): string => attrText(s.attrs.status) || NO_STATUS;

export function groups(rows: readonly Signal[]): Group[] {
	const out = new Map<string, Group>();
	for (const s of rows) {
		const status = statusOf(s);
		const g = out.get(status) ?? { status, count: 0, tone: NEUTRAL };
		g.count++;
		if (g.tone === NEUTRAL) g.tone = toneOf(s.attrs.tone);
		out.set(status, g);
	}
	return [...out.values()];
}

export const matches = (s: Signal, chips: readonly Chip[]): boolean =>
	chips.every((c) => (c.key === "collector" ? s.collector === c.value : attrText(s.attrs[c.key]) === c.value));

export function chipsOf(rows: readonly Signal[]): Chip[] {
	const seen = new Map<string, Chip>();
	for (const s of rows) {
		const candidates: Chip[] = [{ key: "collector", value: s.collector }];
		for (const key of ["status", "tone", "table"]) {
			const value = attrText(s.attrs[key]);
			if (value !== "") candidates.push({ key, value });
		}
		for (const c of candidates) seen.set(`${c.key}=${c.value}`, c);
	}
	return [...seen.values()];
}

export const scopeLine = (shown: number, total: number) => `Filters apply to the latest ${shown} of ${total} events read.`;

export const selectionKey = (s: Signal): string => (s.id ? `${s.collector}/${s.id}` : `seq:${s.seq}`);

export const BODY_LIMIT = 4000;

export function bodyText(s: Signal): { text: string; truncated: boolean } {
	const body = s.event?.body ?? "";
	return body.length > BODY_LIMIT ? { text: body.slice(0, BODY_LIMIT), truncated: true } : { text: body, truncated: false };
}
