export type Attrs = Record<string, unknown>;
export interface Candidate {
	id: string;
	lastUpAt: string;
	finishedAt: string;
	keep: boolean;
	submitter: string;
}
export interface CollectStage {
	status: CollectStageStatus;
	detail?: string;
}
export type CollectStageStatus = "ok" | "skipped" | "absent" | "error";
export interface CollectorState {
	id: string;
	state: State;
	lastOkAt?: string;
	failures: number;
	reason?: string;
}
export interface Count {
	count: number;
	cap: number;
}
export type DestroyReason = "" | "person" | "pool";
export type Env = "absent" | "up" | "stopped" | "destroyed";
export type Event = "submit" | "tick-start" | "prepared" | "built" | "sealed" | "booted" | "tested" | "reported" | "stop" | "stopped" | "resume" | "resumed" | "resume-failed" | "destroy" | "destroyed" | "fail" | "abort" | "reject" | "observed";
export interface EventBody {
	body?: string;
}
export interface ExecFlow {
	id: string;
	steps: ExecStep[];
}
export interface ExecStep {
	id: string;
	status: StepStatus;
	httpStatus?: number;
	message?: string;
}
export interface ExecView {
	exitCode: number | null;
	oomKilled: boolean;
	timedOut: boolean;
	flows: ExecFlow[];
	lines: string[];
	reason?: string;
}
export type FlowOutcome = "passed" | "failed" | "skipped" | "notRun" | "incomplete";
export interface FlowStep {
	id: string;
	title: string;
	kind: StepKind;
	status: StepStatus;
	requests: number;
	durationMs?: number;
	httpStatus?: number;
	message?: string;
	source?: ManifestSource;
}
export interface FlowSummary {
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
}
export interface FlowView {
	source: string;
	flow: FlowSummary;
}
export interface HubList {
	runs: RunItem[];
	orphans: Orphan[];
	pool: Size;
	up: Count;
	dockerDown: boolean;
	tickError: string | null;
	servedHostApis: number[];
}
export interface JobEvent {
	jobId: string;
	title: string;
	phase: JobPhase;
	text?: string;
	code?: number;
}
export type JobPhase = "start" | "out" | "exit";
export interface KitExec {
	command: string;
	flows: string[];
}
export interface KitFiles {
	dir: string;
	readme: string;
}
export interface KitView {
	id: string;
	env: Env;
	services: ServicePort[];
	kit: KitFiles | null;
	exec: KitExec | null;
	reportDir: string;
}
export interface ManifestSource {
	file: string;
	line: number;
}
export type Mark = "" | "lost" | "orphan";
export interface MetricBody {
	name: string;
	value: number;
}
export interface NotProven {
	skipped: string[];
	declared: string[];
	unread: string[];
}
export interface OpReply {
	run: RunItem;
}
export type OpenMode = "browser" | "link" | "copy";
export interface Orphan {
	project: string;
	running: boolean;
}
export type Phase = "queued" | "preparing" | "building" | "sealing" | "booting" | "testing" | "reporting" | "stopping" | "resuming" | "destroying" | "done";
export interface Problem {
	code: string;
	message: string;
	files?: string[];
	run?: RunItem;
	candidates?: Candidate[];
}
export interface ReadResponse {
	signals: Signal[];
	frozen: boolean;
	frozenSeq: number;
	dropped: Record<string, number>;
	resync?: Resync;
	next: number;
}
export interface Redaction {
	status: string;
	files: string[];
}
export interface ReportView {
	id: string;
	phase: Phase;
	phaseLabel: string;
	verdict: Verdict | null;
	verdictLabel: string | null;
	status: string;
	files?: string[];
	failedStage?: Phase;
	notice?: string;
	tail?: string[];
	summary?: unknown;
	meta?: unknown;
	flows?: FlowSummary[];
	steps?: StepCounts;
	stoppedAt?: SummaryStop;
	notProven?: NotProven;
	reportDir?: string;
}
export interface Resync {
	from: number;
	to: number;
}
export interface RunDetail extends RunItem {
	imageKeys?: Record<string, string>;
	imageDigests?: Record<string, string>;
	runnerImage?: RunnerImage;
	tail?: string[];
	allowedEvents: Event[];
	services: ServicePort[];
}
export interface RunItem {
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
	redaction?: Redaction;
	phaseLabel: string;
	verdictLabel: string | null;
}
export interface RunnerImage {
	tag: string;
	id: string;
}
export interface ServicePort {
	name: string;
	service: string;
	port: number;
	hostPort: number;
	label: string;
	open?: OpenMode;
}
export interface Signal {
	seq: number;
	ts: string;
	tsSource: TimeSource;
	kind: SignalKind;
	collector: string;
	id?: string;
	rev?: number;
	attrs: Attrs;
	metric?: MetricBody;
	event?: EventBody;
	span?: SpanBody;
}
export type SignalKind = "metric" | "event" | "span";
export interface SignalsView extends ReadResponse {
	collectors: CollectorState[];
}
export interface Size {
	size: number;
	max: number;
}
export interface SourceView {
	source: string;
	file: string;
	lines: number;
	text: string;
}
export interface SpanBody {
	traceId: string;
	parentId?: string;
	name: string;
	start: string;
	end?: string;
	status: SpanStatus;
	links?: SpanLink[];
}
export interface SpanLink {
	traceId: string;
	spanId: string;
}
export type SpanStatus = "ok" | "error" | "incomplete";
export type State = "armed" | "backoff" | "stopped" | "report-only";
export interface StepCounts {
	expected?: number;
	actual?: number;
	failed: number;
	skipped: number;
	requests: number;
}
export type StepKind = "request" | "wait" | "capture";
export type StepStatus = "notRun" | "running" | "passed" | "failed" | "skipped";
export type StopReason = "" | "person" | "cap" | "done";
export interface Submitted {
	id: string;
	phase: Phase;
	position: number;
	merged: boolean;
}
export interface SuiteName {
	name: string;
}
export interface SuitesView {
	worktree: string;
	suites: SuiteName[];
}
export interface SummaryStop {
	flow: string;
	step: string;
	message?: string;
}
export type TimeSource = "host" | "container" | "runner";
export type Tone = "ok" | "warn" | "bad" | "idle" | "info";
export type Verdict = "passed" | "failed" | "error" | "aborted";
export type WaitReason = "" | "active-run" | "pool-all-kept" | "pool-over" | "in-progress" | "docker-unavailable" | "no-port-block";
