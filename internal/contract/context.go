package contract

type Context struct {
	SchemaVersion SchemaVersion     `json:"schemaVersion"`
	Captures      map[string]string `json:"captures"`
	TOTPSteps     map[string]int64  `json:"totpSteps"`
}

func (c *Context) schemaVersion() SchemaVersion { return c.SchemaVersion }
