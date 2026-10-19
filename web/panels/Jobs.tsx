import { useEffect, useRef } from "react";
import type { JobLog } from "../store";
import { Badge, Panel } from "../ui";

export function Jobs({ jobs, empty }: { jobs: JobLog[]; empty: string }) {
	const latest = jobs.at(-1);
	const end = useRef<HTMLDivElement>(null);
	useEffect(() => {
		end.current?.scrollIntoView({ block: "end" });
	}, [latest?.lines.length]);
	if (!latest) return <p className="muted">{empty}</p>;
	return (
		<Panel
			title={latest.title}
			actions={latest.running ? <Badge tone="warn">running</Badge> : <Badge tone={latest.code === 0 ? "ok" : "bad"}>exit {latest.code ?? "-"}</Badge>}
		>
			<pre className="console">
				{latest.lines.join("\n")}
				<div ref={end} />
			</pre>
		</Panel>
	);
}
