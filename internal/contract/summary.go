package contract

type SummaryFormat int

const SummaryFormat3 SummaryFormat = 3

type FinalVerdict string

const (
	VerdictPassed  FinalVerdict = "passed"
	VerdictFailed  FinalVerdict = "failed"
	VerdictError   FinalVerdict = "error"
	VerdictAborted FinalVerdict = "aborted"
)

type SealVerdict string

const (
	SealPassed SealVerdict = "passed"
	SealFailed SealVerdict = "failed"
	SealError  SealVerdict = "error"
)

type FlowOutcome string

const (
	FlowOutcomePassed     FlowOutcome = "passed"
	FlowOutcomeFailed     FlowOutcome = "failed"
	FlowOutcomeSkipped    FlowOutcome = "skipped"
	FlowOutcomeNotRun     FlowOutcome = "notRun"
	FlowOutcomeIncomplete FlowOutcome = "incomplete"
)

type CollectStageStatus string

const (
	CollectStageOK      CollectStageStatus = "ok"
	CollectStageSkipped CollectStageStatus = "skipped"
	CollectStageAbsent  CollectStageStatus = "absent"
	CollectStageError   CollectStageStatus = "error"
)

type StepCounts struct {
	Expected *int `json:"expected,omitempty"`
	Actual   *int `json:"actual,omitempty"`
	Failed   int  `json:"failed"`
	Skipped  int  `json:"skipped"`
	Requests int  `json:"requests"`
}

type FlowStep struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Kind       StepKind        `json:"kind"`
	Status     StepStatus      `json:"status"`
	Requests   int             `json:"requests"`
	DurationMs *int64          `json:"durationMs,omitempty"`
	HTTPStatus *int            `json:"httpStatus,omitempty"`
	Message    string          `json:"message,omitempty"`
	Source     *ManifestSource `json:"source,omitempty"`
	Calls      []StepCall      `json:"calls,omitempty"`
	Refs       []StepRef       `json:"refs,omitempty"`
}

type FlowSummary struct {
	ID        string          `json:"id"`
	Stage     string          `json:"stage"`
	Title     string          `json:"title"`
	Once      bool            `json:"once"`
	Requires  []string        `json:"requires"`
	Source    *ManifestSource `json:"source,omitempty"`
	StepList  []FlowStep      `json:"stepList"`
	Status    FlowOutcome     `json:"status"`
	Steps     int             `json:"steps"`
	Passed    int             `json:"passed"`
	Failed    int             `json:"failed"`
	Skipped   int             `json:"skipped"`
	StoppedAt string          `json:"stoppedAt,omitempty"`
}

type SummaryStop struct {
	Flow    string `json:"flow"`
	Step    string `json:"step"`
	Message string `json:"message,omitempty"`
}

type NotProven struct {
	Skipped  []string `json:"skipped"`
	Declared []string `json:"declared"`
	Unread   []string `json:"unread"`
}

type BootVerdict struct {
	OK      bool     `json:"ok"`
	Pending *bool    `json:"pending,omitempty"`
	Waiting []string `json:"waiting,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
}

type RunnerOutcome struct {
	ContainerID string   `json:"containerId"`
	Code        *int     `json:"code,omitempty"`
	OOMKilled   bool     `json:"oomKilled"`
	TimedOut    bool     `json:"timedOut"`
	Aborted     bool     `json:"aborted"`
	Lines       []string `json:"lines"`
}

type CollectStage struct {
	Status CollectStageStatus `json:"status"`
	Detail string             `json:"detail,omitempty"`
}

type SummaryImages struct {
	Keys    map[string]string `json:"keys"`
	Digests map[string]string `json:"digests"`
}

type RunnerImage struct {
	Tag string `json:"tag"`
	ID  string `json:"id"`
}

type Summary struct {
	Format         SummaryFormat           `json:"format"`
	RunID          string                  `json:"runId"`
	Suite          string                  `json:"suite"`
	Verdict        FinalVerdict            `json:"verdict"`
	Reasons        []string                `json:"reasons"`
	Steps          StepCounts              `json:"steps"`
	Flows          []FlowSummary           `json:"flows"`
	StoppedAt      *SummaryStop            `json:"stoppedAt,omitempty"`
	NotProven      NotProven               `json:"notProven"`
	Boot           *BootVerdict            `json:"boot,omitempty"`
	Seal           SealVerdict             `json:"seal"`
	Runner         *RunnerOutcome          `json:"runner,omitempty"`
	Stages         map[string]CollectStage `json:"stages"`
	Commit         string                  `json:"commit"`
	Harness        int                     `json:"harness"`
	HubCommit      string                  `json:"hubCommit"`
	Ports          map[string]int          `json:"ports"`
	Images         SummaryImages           `json:"images"`
	Versions       map[string]string       `json:"versions"`
	CockpitVersion string                  `json:"cockpitVersion"`
	HostAPI        int                     `json:"hostApi"`
	RunnerImage    RunnerImage             `json:"runnerImage"`
	GeneratedAt    string                  `json:"generatedAt"`
}
