import { serviceTone } from "../shared/classify";
import type { ServicePort, Signal } from "../shared/types";
import { attrText, SERVICES_TABLE } from "../signals/model";
import { Badge, Copy, Panel } from "../ui";

export type ServiceStatus = { status: string; health: string };

export function serviceStates(signals: readonly Signal[]): Map<string, ServiceStatus> {
	const out = new Map<string, ServiceStatus>();
	for (const s of signals) {
		if (s.kind !== "event" || attrText(s.attrs.table) !== SERVICES_TABLE) continue;
		const name = attrText(s.attrs.label);
		if (name !== "") out.set(name, { status: attrText(s.attrs.status), health: attrText(s.attrs.health) });
	}
	return out;
}

export function RunServices({ id, ports, signals }: { id: string; ports: ServicePort[] | null; signals: readonly Signal[] }) {
	const states = serviceStates(signals);
	return (
		<Panel title={`Port map ${ports?.length ?? 0}`}>
			{ports === null ? <p className="muted">Loading</p> : null}
			{ports?.length === 0 ? <p className="muted">No port block. Only runs with an environment hold ports.</p> : null}
			<ul className="services">
				{ports?.map((p) => {
					const s = states.get(p.service);
					return (
						<li key={p.name}>
							{s ? <Badge tone={serviceTone(s.status, s.health || undefined)}>{s.health || s.status}</Badge> : null}
							<span className="name" title={`${p.service}:${p.port}`}>
								{p.label}
							</span>
							<Address id={id} port={p} />
						</li>
					);
				})}
			</ul>
		</Panel>
	);
}

export function addressOf(id: string, port: ServicePort): { href: string | null; text: string } {
	const local = `127.0.0.1:${port.hostPort}`;
	const open = port.open ?? "link";
	if (open === "copy") return { href: null, text: local };
	const href = open === "browser" ? `http://${id}.localhost:${port.hostPort}` : `http://${local}`;
	return { href, text: href.slice("http://".length) };
}

function Address({ id, port }: { id: string; port: ServicePort }) {
	const address = addressOf(id, port);
	if (address.href === null) return <Copy text={address.text} />;
	return (
		<a href={address.href} target="_blank" rel="noreferrer" className="port">
			{address.text}
		</a>
	);
}
