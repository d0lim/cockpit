package report

import (
	"github.com/d0lim/cockpit/internal/contract"
)

type ruleCase struct {
	name    string
	in      Inputs
	verdict contract.FinalVerdict
	rule    int
	reason  string
}

func ruleCases() []ruleCase {
	pending := true
	return []ruleCase{
		{"a seal violation wins even with no boot, manifest or exit code", with(func(in *Inputs) {
			in.Seal, in.Boot, in.Manifest, in.RunnerCode = contract.SealFailed, nil, ManifestRead{Failure: ReadMissing}, nil
		}), vFailed, 1, "a seal violation exists"},
		{"an undecided seal is an error", with(func(in *Inputs) { in.Seal = contract.SealError }), vError, 1, "the seal check could not decide"},
		{"a pending boot names the first waiting service before the memory kill", with(func(in *Inputs) {
			in.Boot, in.OOMKilled = &contract.BootVerdict{Pending: &pending, Waiting: []string{"app: starting", "db: starting"}}, true
		}), vError, 2, "the boot check failed: app: starting"},
		{"a missing boot verdict is an error", with(func(in *Inputs) { in.Boot = nil }), vError, 2, "no boot verdict"},
		{"a memory kill comes before the missing exit code", with(func(in *Inputs) { in.OOMKilled, in.RunnerCode = true, nil }), vError, 3, "OOMKilled"},
		{"a missing exit code comes before the missing manifest", with(func(in *Inputs) {
			in.RunnerCode, in.Manifest = nil, ManifestRead{Failure: ReadMissing}
		}), vError, 4, "no runner exit code"},
		{"exit code 137 is named and comes before the missing manifest", with(func(in *Inputs) {
			in.RunnerCode, in.Manifest = intp(137), ManifestRead{Failure: ReadMissing}
		}), vError, 5, "runner exit code 137: SIGKILL or OOM"},
		{"exit code 9 is outside the contract", with(func(in *Inputs) { in.RunnerCode = intp(9) }), vError, 5, "outside the contract (0, 1, 2)"},
		{"a missing manifest comes before the missing result", with(func(in *Inputs) {
			in.Manifest, in.Result = ManifestRead{Failure: ReadMissing}, ResultRead{Failure: ReadMissing}
		}), vError, 6, "the manifest is missing"},
		{"a manifest that fails the schema is an error", with(func(in *Inputs) { in.Manifest = ManifestRead{Failure: ReadSchema} }), vError, 6, "the manifest does not match the schema"},
		{"an unfinished result comes before the step-set mismatch", with(func(in *Inputs) {
			in.Result.Value.Complete = false
			in.Result.Value.Steps = in.Result.Value.Steps[1:]
		}), vError, 7, "complete: false"},
		{"a result that is not JSON is an error", with(func(in *Inputs) { in.Result = ResultRead{Failure: ReadParse} }), vError, 7, "the result is not JSON"},
		{"a step-set mismatch comes before the failed step", with(func(in *Inputs) {
			in.Result.Value, in.RunnerCode = failedAtA2("boom"), intp(1)
			in.Result.Value.Steps = in.Result.Value.Steps[:len(in.Result.Value.Steps)-1]
		}), vError, 8, "the step counts differ: result 5, manifest 6"},
		{"a repeated result step id is an error", with(func(in *Inputs) { in.Result.Value.Steps[1] = in.Result.Value.Steps[0] }), vError, 8, "a step id repeats in the result: alpha › a1"},
		{"a result step outside the manifest is an error", with(func(in *Inputs) { in.Result.Value.Steps[0].ID = "ghost" }), vError, 8, "not in the manifest: alpha › ghost"},
		{"a manifest without flows but a result with steps is a mismatch", with(func(in *Inputs) { in.Manifest.Value.Flows = nil }), vError, 8, "the step counts differ: result 6, manifest 0"},
		{"a manifest without flows and a result without steps is an error", with(func(in *Inputs) {
			in.Manifest.Value.Flows, in.Result.Value.Steps = nil, nil
		}), vError, 9, "the manifest has no flows"},
		{"a step left notRun comes before the vacuous step", with(func(in *Inputs) {
			step(in.Result.Value, "alpha", "a3").Status = contract.StepStatusNotRun
			step(in.Result.Value, "gamma", "g1").Requests = 0
		}), vError, 10, "a step did not run in a finished result: alpha › a3"},
		{"a passed request step with zero requests comes before the exit-code mismatch", with(func(in *Inputs) {
			in.Result.Value = failedAtA2("boom")
			step(in.Result.Value, "gamma", "g1").Requests = 0
		}), vError, 11, "no request was sent: gamma › g1"},
		{"exit code 0 with a failed step comes before the failed step", with(func(in *Inputs) { in.Result.Value = failedAtA2("boom") }), vError, 12, "exit code 0 with failed or skipped steps"},
		{"a failed step comes before the wrong prerequisite", with(func(in *Inputs) {
			in.Result.Value, in.RunnerCode = failedAtA2("boom"), intp(1)
			*step(in.Result.Value, "gamma", "g1") = contract.ResultStep{Flow: "gamma", ID: "g1", Status: contract.StepStatusSkipped}
		}), vFailed, 13, "failed steps: 1, the first at alpha › a2"},
		{"a skip without a failed ancestor comes before exit code 2", with(func(in *Inputs) {
			in.RunnerCode = intp(2)
			*step(in.Result.Value, "gamma", "g1") = contract.ResultStep{Flow: "gamma", ID: "g1", Status: contract.StepStatusSkipped}
		}), vError, 14, "wrong prerequisite: gamma › g1"},
		{"exit code 2 is a runner internal error", with(func(in *Inputs) { in.RunnerCode = intp(2) }), vError, 15, "runner internal error"},
		{"exit code 1 with nothing failed or skipped is an error", with(func(in *Inputs) { in.RunnerCode = intp(1) }), vError, 15, "exit code 1 with no failed or skipped steps"},
	}
}
