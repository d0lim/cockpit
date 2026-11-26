package contract

type Context struct {
	SchemaVersion SchemaVersion     `json:"schemaVersion"`
	Captures      map[string]string `json:"captures"`
	TOTPSteps     map[string]int64  `json:"totpSteps"`
	TOTPStep      int64             `json:"totpStepMillis,omitempty"`
}

const StandardTOTPStepMillis int64 = 30_000

func (c *Context) schemaVersion() SchemaVersion { return c.SchemaVersion }
