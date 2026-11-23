package contract

type ArchiveFormat int

const ArchiveFormat1 ArchiveFormat = 1

type ArchiveKind string

const (
	ArchiveKindSummary     ArchiveKind = "summary"
	ArchiveKindMeta        ArchiveKind = "meta"
	ArchiveKindNotProven   ArchiveKind = "not-proven"
	ArchiveKindSeal        ArchiveKind = "seal"
	ArchiveKindManifest    ArchiveKind = "manifest"
	ArchiveKindResult      ArchiveKind = "result"
	ArchiveKindLedger      ArchiveKind = "ledger"
	ArchiveKindContext     ArchiveKind = "context"
	ArchiveKindExec        ArchiveKind = "exec"
	ArchiveKindSourceIndex ArchiveKind = "source-index"
	ArchiveKindSource      ArchiveKind = "source"
	ArchiveKindSignals     ArchiveKind = "signals"
	ArchiveKindWireIndex   ArchiveKind = "wire-index"
	ArchiveKindWireCalls   ArchiveKind = "wire-calls"
	ArchiveKindSchema      ArchiveKind = "schema"
)

type ArchiveFile struct {
	Path   string      `json:"path"`
	Kind   ArchiveKind `json:"kind"`
	Bytes  int64       `json:"bytes"`
	Lines  int         `json:"lines,omitempty"`
	Schema string      `json:"schema,omitempty"`
}

type Archive struct {
	Format         ArchiveFormat `json:"format"`
	RunID          string        `json:"runId"`
	ExportedAt     string        `json:"exportedAt"`
	CockpitVersion string        `json:"cockpitVersion"`
	Files          []ArchiveFile `json:"files"`
	Omitted        []string      `json:"omitted"`
}
