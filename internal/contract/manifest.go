package contract

type SchemaVersion int

const (
	SchemaVersion1 SchemaVersion = 1
	SchemaVersion2 SchemaVersion = 2
)

const RunnerSchemaVersion = SchemaVersion2

type StepKind string

const (
	StepKindRequest StepKind = "request"
	StepKindWait    StepKind = "wait"
	StepKindCapture StepKind = "capture"
)

type ManifestSource struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type ManifestStage struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	NotProven string `json:"notProven,omitempty"`
}

type ManifestStep struct {
	ID     string          `json:"id"`
	Title  string          `json:"title"`
	Kind   StepKind        `json:"kind"`
	Source *ManifestSource `json:"source,omitempty"`
}

type ManifestFlow struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Stage    string          `json:"stage"`
	Once     bool            `json:"once"`
	Requires []string        `json:"requires"`
	Steps    []ManifestStep  `json:"steps"`
	Source   *ManifestSource `json:"source,omitempty"`
}

type RunnerIdentity struct {
	Version string `json:"version"`
	Image   string `json:"image"`
}

type Manifest struct {
	SchemaVersion SchemaVersion   `json:"schemaVersion"`
	Suite         string          `json:"suite"`
	Stages        []ManifestStage `json:"stages"`
	Flows         []ManifestFlow  `json:"flows"`
	Runner        *RunnerIdentity `json:"runner,omitempty"`
}

func (m *Manifest) schemaVersion() SchemaVersion { return m.SchemaVersion }
