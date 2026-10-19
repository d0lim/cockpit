export type Phase =
	| "queued"
	| "preparing"
	| "building"
	| "sealing"
	| "booting"
	| "testing"
	| "reporting"
	| "stopping"
	| "resuming"
	| "destroying"
	| "done";
export type Env = "absent" | "up" | "stopped" | "destroyed";
export type Verdict = "passed" | "failed" | "error" | "aborted";
export type Mark = "lost" | "orphan";
export type WaitReason = "active-run" | "pool-all-kept" | "pool-over" | "in-progress" | "docker-unavailable" | "no-port-block";
export type StopReason = "person" | "cap" | "done";
export type DestroyReason = "person" | "pool";
export type RunOp = "keep" | "unkeep" | "stop" | "resume" | "destroy" | "abort";
export type RunEvent = "stop" | "resume" | "destroy" | "abort";
export type OpenMode = "browser" | "link" | "copy";

export type RunItem = {
	id: string;
	phase: Phase;
	env: Env;
	verdict: Verdict | null;
	mark?: Mark;
	sha: string;
	suite: string;
	hostApi: number;
	cockpitVersion: string;
	submitter: string;
	keep: boolean;
	createdAt: string;
	startedAt?: string;
	finishedAt?: string;
	lastUpAt?: string;
	slot?: number;
	position?: number;
	waitReason?: WaitReason;
	stopReason?: StopReason;
	destroyReason?: DestroyReason;
	failedStage?: Phase;
	notice?: string;
	recreate?: boolean;
	redaction?: { status: string; files: string[] };
	phaseLabel: string;
	verdictLabel: string | null;
};

export type ServicePort = { name: string; service: string; port: number; hostPort: number; label: string; open?: OpenMode };

export type RunDetail = RunItem & {
	imageKeys?: Record<string, string>;
	imageDigests?: Record<string, string>;
	runnerImage?: { tag: string; id: string };
	tail?: string[];
	allowedEvents: RunEvent[];
	services: ServicePort[];
};

export type Orphan = { project: string; running: boolean };

export type HubList = {
	runs: RunItem[];
	orphans: Orphan[];
	pool: { size: number; max: number };
	up: { count: number; cap: number };
	dockerDown: boolean;
	tickError: string | null;
	servedHostApis?: number[];
};

export type Candidate = { id: string; lastUpAt: string; finishedAt: string; keep: boolean; submitter: string };

export type Submitted = { id: string; phase: Phase; position: number; merged: boolean };

export type SuitesView = { worktree: string; suites: { name: string }[] };

export type KitView = {
	id: string;
	env: Env;
	services: ServicePort[];
	kit: { dir: string; readme: string } | null;
	exec: { command: string; flows: string[] } | null;
	reportDir: string;
};

export type StepKind = "request" | "wait" | "capture";
export type StepStatus = "passed" | "failed" | "skipped" | "incomplete" | "running" | "notRun";
export type FlowOutcome = "passed" | "failed" | "skipped" | "notRun" | "incomplete";
export type ManifestSource = { file: string; line: number };

export type FlowStep = {
	id: string;
	title: string;
	kind: StepKind;
	status: StepStatus;
	requests: number;
	durationMs?: number;
	httpStatus?: number;
	message?: string;
	source?: ManifestSource;
};

export type FlowSummary = {
	id: string;
	stage: string;
	title: string;
	once: boolean;
	requires: string[];
	source?: ManifestSource;
	stepList: FlowStep[];
	status: FlowOutcome;
	steps: number;
	passed: number;
	failed: number;
	skipped: number;
	stoppedAt?: string;
};

export type StepCounts = { expected?: number; actual?: number; failed: number; skipped: number; requests: number };
export type StopPoint = { flow: string; step: string; message?: string };
export type NotProven = { skipped: string[]; declared: string[]; unread: string[] };
export type CollectStage = { status: "ok" | "skipped" | "absent" | "error"; detail?: string };

type ReportBase = { id: string; phase: Phase; phaseLabel: string; verdict: Verdict | null; verdictLabel: string | null };
export type ReportView = ReportBase &
	(
		| { status: "pending" }
		| { status: "redaction-failed"; files: string[] }
		| { status: "meta-only"; failedStage?: Phase; notice?: string; tail?: string[] }
		| {
				status: "bundle";
				summary: unknown;
				meta: unknown;
				flows?: FlowSummary[];
				steps?: StepCounts;
				stoppedAt?: StopPoint;
				notProven?: NotProven;
				reportDir: string;
		  }
	);

export type ExecView = {
	exitCode: number | null;
	oomKilled: boolean;
	timedOut: boolean;
	flows: { id: string; steps: { id: string; status: StepStatus; httpStatus?: number; message?: string }[] }[];
	lines: string[];
	reason?: string;
};

export type DataSource = "live" | "bundle";
export type FlowView = { source: DataSource; flow: FlowSummary };
export type SourceView = { source: DataSource; file: string; lines: number; text: string };

export type Tone = "ok" | "warn" | "bad" | "idle" | "info";
export type TimeSource = "host" | "container" | "runner";
export type SpanStatus = "ok" | "error" | "incomplete";
export type AttrValue = string | number | boolean;

export type Signal = {
	seq: number;
	ts: string;
	tsSource: TimeSource;
	kind: "metric" | "event" | "span";
	collector: string;
	id?: string;
	rev?: number;
	attrs: Record<string, AttrValue>;
	metric?: { name: string; value: number };
	event?: { body?: string };
	span?: { traceId: string; parentId?: string; name: string; start: string; end?: string; status: SpanStatus; links?: { traceId: string; spanId: string }[] };
};

export type Resync = { from: number; to: number };

export type CollectorState = { id: string; state: string; lastOkAt?: string; failures: number; reason?: string };

export type SignalsView = {
	signals: Signal[];
	frozen: boolean;
	frozenSeq: number;
	dropped: Record<string, number>;
	resync?: Resync;
	next: number;
	collectors: CollectorState[];
};

export type JobEvent = { kind: "job"; target: string; at: string; jobId: string; title: string; phase: "start" | "out" | "exit"; text?: string; code?: number };
export type RunsEvent = { kind: "runs"; target: string; at: string; list: HubList };
