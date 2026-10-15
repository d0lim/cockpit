package contract

type StepStatus string

const (
	StepStatusNotRun  StepStatus = "notRun"
	StepStatusRunning StepStatus = "running"
	StepStatusPassed  StepStatus = "passed"
	StepStatusFailed  StepStatus = "failed"
	StepStatusSkipped StepStatus = "skipped"
)

type ResultStep struct {
	Flow       string     `json:"flow"`
	ID         string     `json:"id"`
	Status     StepStatus `json:"status"`
	DurationMs *int64     `json:"durationMs,omitempty"`
	Requests   int        `json:"requests"`
	HTTPStatus *int       `json:"httpStatus,omitempty"`
	Message    string     `json:"message,omitempty"`
	StartedAt  string     `json:"startedAt,omitempty"`
	FinishedAt string     `json:"finishedAt,omitempty"`
}

type StopPoint struct {
	Flow string `json:"flow"`
	Step string `json:"step"`
}

type Result struct {
	SchemaVersion SchemaVersion `json:"schemaVersion"`
	Suite         string        `json:"suite"`
	Complete      bool          `json:"complete"`
	StartedAt     string        `json:"startedAt"`
	FinishedAt    string        `json:"finishedAt,omitempty"`
	StoppedAt     *StopPoint    `json:"stoppedAt,omitempty"`
	Steps         []ResultStep  `json:"steps"`
}

func (r *Result) schemaVersion() SchemaVersion { return r.SchemaVersion }
