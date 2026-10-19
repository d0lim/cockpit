import { type ReactNode, useState } from "react";
import type { NEUTRAL } from "./shared/classify";
import type { Tone } from "./shared/types";

export function Panel({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
	return (
		<section className="panel">
			<header className="panel-head">
				<h2>{title}</h2>
				{actions}
			</header>
			<div className="panel-body">{children}</div>
		</section>
	);
}

export const Badge = ({ tone, children }: { tone: Tone | typeof NEUTRAL; children: ReactNode }) => <span className={`badge ${tone}`}>{children}</span>;

const pad = (n: number) => String(n).padStart(2, "0");

export function time(iso: string | null | undefined): string {
	if (!iso) return "";
	const d = new Date(iso);
	return Number.isNaN(d.getTime()) ? "" : `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

export function stamp(iso: string | null | undefined): string {
	if (!iso) return "-";
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return "-";
	return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function Copy({ text, children }: { text: string; children?: ReactNode }) {
	const [done, setDone] = useState(false);
	const copy = () =>
		navigator.clipboard.writeText(text).then(
			() => {
				setDone(true);
				setTimeout(() => setDone(false), 1200);
			},
			() => setDone(false),
		);
	return (
		<button type="button" className="link" title="Copy" onClick={copy}>
			{children ?? text}
			{done ? " ✓" : ""}
		</button>
	);
}

export const Notice = ({ children }: { children: ReactNode }) => <p className="notice">{children}</p>;
