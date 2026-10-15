package contract

type ExecResult struct {
	Result `tstype:",extends"`
	Target string `json:"target"`
}
