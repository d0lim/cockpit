package contract

type FlowStatus string

const (
	FlowStatusStarted FlowStatus = "started"
	FlowStatusPassed  FlowStatus = "passed"
	FlowStatusFailed  FlowStatus = "failed"
	FlowStatusSkipped FlowStatus = "skipped"
)

type LedgerEntry struct {
	Status FlowStatus `json:"status"`
	At     string     `json:"at"`
}

type Ledger struct {
	SchemaVersion SchemaVersion          `json:"schemaVersion"`
	Flows         map[string]LedgerEntry `json:"flows"`
}

func (l *Ledger) schemaVersion() SchemaVersion { return l.SchemaVersion }
