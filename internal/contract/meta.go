package contract

type BundleMeta struct {
	ID             string            `json:"id"`
	SHA            string            `json:"sha"`
	Suite          string            `json:"suite"`
	Harness        int               `json:"harness"`
	HubCommit      string            `json:"hubCommit"`
	Submitter      string            `json:"submitter"`
	Keep           bool              `json:"keep"`
	CreatedAt      string            `json:"createdAt"`
	StartedAt      string            `json:"startedAt,omitempty"`
	FinishedAt     string            `json:"finishedAt,omitempty"`
	Slot           *int              `json:"slot,omitempty"`
	ImageKeys      map[string]string `json:"imageKeys,omitempty"`
	ImageDigests   map[string]string `json:"imageDigests,omitempty"`
	FailedStage    string            `json:"failedStage,omitempty"`
	Notice         string            `json:"notice,omitempty"`
	Tail           []string          `json:"tail,omitempty"`
	Verdict        FinalVerdict      `json:"verdict"`
	ReportedAt     string            `json:"reportedAt"`
	CockpitVersion string            `json:"cockpitVersion"`
	HostAPI        int               `json:"hostApi"`
	RunnerImage    RunnerImage       `json:"runnerImage"`
}
