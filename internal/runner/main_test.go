package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var runnerMutations = []porttest.Mutation{
	{"hostApi mismatch exits 2 before the manifest", "delete the hostApi comparison in Main", "red: exit 0 and manifest.json is written", "TestHostAPIMismatchExitsTwoBeforeManifestAndNamesBothValues"},
	{"missing declaration is refused", "default hostApi to the runner value when the declaration file is missing", "red: exit 0 without a declaration", "TestMissingOrUnreadableDeclarationExitsTwo"},
	{"once flow that already ran is refused", "delete the ran check in PlanExec", "red: alpha runs again and the exit is 0", "TestAE3OnceFlowWithLedgerRecordDoesNotRunAndExitsTwoWithReason"},
	{"unpassed once require is refused", "delete the passed check in PlanExec", "red: requests go out instead of exit 2", "TestAE4OnceRequireNotPassedInLedgerExitsTwoAndSendsNothing"},
	{"failure message masking", "Finish writes the raw message instead of calling Summarize", "red: the planted token shows up in result.json", "TestStdoutStderrAndFilesHidePlantedSecretsAndKeepMaskedFailureMessage"},
	{"console output masking", "consoleHandler writes without masking", "red: the private value shows up on stderr", "TestStdoutStderrAndFilesHidePlantedSecretsAndKeepMaskedFailureMessage"},
	{"mask before truncating", "truncate to 500 characters before Summarize", "red: the cut fragment keeps the start of the private value", "TestFailureMessageIsMaskedBeforeTruncationSoNoSecretFragmentSurvives"},
	{"skip does not overwrite an earlier record", "delete the skipped guard in setLedger", "red: two is overwritten with skipped", "TestSkipDoesNotOverwriteEarlierLedgerRecordSinceItOnlyMeansNotStarted"},
	{"a failed require blocks later flows", "delete the blocked check in Next", "red: beta runs and sends requests", "TestSkipFollowsRequiresSoGrandchildOfFailedFlowNeverStarts"},
	{"atomic writes", "WriteJSON writes the target file directly", "red: a .tmp file shows up mid-step or the JSON is broken", "TestAE7MidStepResultIsValidJSONIncompleteWithRunningStepAndStartedLedger"},
	{"exec does not touch the full-run files", "exec plans also write result.json", "red: result.json is created", "TestAE4ExecRerunsRepeatableRequiresAndReusesPassedOnceRequireOnEveryCall"},
	{"later targets are skipped after an earlier target fails", "set skipAll to false always", "red: three runs", "TestThreeTargetsRunInOrderAndSecondFailureSkipsThirdWithThreeExecFiles"},
	{"only an all-passing run exits 0", "ExitCode always returns 0", "red: a failing run exits 0", "TestAE2FailingStepSkipsRemainingStepsAndDependentFlowsAndExitsOne"},
	{"contract shape", "drop runner from the manifest or set the result schemaVersion to 1", "red: the assertions and schema validation fail", "TestAE1AllPassingSuiteWritesMatchingStepsCompleteResultPassedLedgerAndExitsZero"},
	{"the Go port counts requests", "Finish pins requests to 1", "red: the noop step has nonzero requests", "TestStepWithoutRequestsRecordsZeroRequestsAndNoHTTPStatus"},
	{"a missing require capture fails the step", "ctx.get returns an empty string", "red: the exit is not 1", "TestOnceRequireCaptureMissingFromContextFailsTheStepThatReadsIt"},
	{"a target whose requires are all passed once flows runs alone", "PlanExec reruns once requires too", "red: an alpha request goes out", "TestTargetWhoseOnlyRequireIsPassedOnceFlowRunsAlone"},
	{"default exec path", "ExecOutPath returns result.json for an empty base", "red: exec/ has no file", "TestExecWithoutE2EExecOutWritesTimestampedFlowFileUnderExecDir"},
	{"a never-started once flow is allowed", "PlanExec also refuses a once flow with no record", "red: exit 2", "TestNeverStartedOnceFlowRunsViaExecAndLedgerBecomesPassed"},
	{"argument checks", "parseArgs skips the pattern checks", "red: a malformed id is accepted", "TestBadArgumentsExitTwoWithReason"},
	{"suite load failure exits 2", "swallow the bundle error and exit 0", "red: exit 0", "TestMissingOrBrokenSuiteFileExitsTwoWithoutManifest"},
	{"suite name mismatch is refused", "delete the name check in Begin", "red: a suite with another name runs", "TestMissingOrBrokenSuiteFileExitsTwoWithoutManifest"},
	{"flow helpers are exported by cockpit:flow", "drop target, expectStatus, json and poll from the module exports", "red: the alpha step fails on an undefined function", "TestFlowHelpersExpectStatusJsonTargetAndPollWorkInsideAStep"},
	{"expectStatus keeps the query and the body out of its message", "append res.url or res.body to the thrown message", "red: the beta message carries the query or the body", "TestExpectStatusFailureNamesMethodAndPathOnly"},
	{"json keeps the body out of its message", "append res.body to the thrown message", "red: the gamma message carries the body", "TestJsonFailureOnANonJSONBodyKeepsTheBodyOut"},
	{"recorded request paths are masked", "takeRecords copies the raw path instead of masking it", "red: the private value shows up in result.json", "TestEachStepRecordsItsRequestsWithMaskedPathsAndItsRefs"},
	{"a captured value stays masked in recorded paths unless a ref names it", "KnownValues leaves every capture out", "red: hidden-id-1 shows up in result.json", "TestEachStepRecordsItsRequestsWithMaskedPathsAndItsRefs"},
	{"ref refuses private values and credential shapes", "Secret answers false", "red: the refuse step fails with refused 0", "TestARefOfAPrivateValueOrACredentialShapeIsRefusedAndAShortPrivateValueBlocksNothing"},
	{"a private value shorter than the masker minimum blocks no ref", "Secret refuses any value containing a private value", "red: the refuse step fails on id-5432-ab", "TestARefOfAPrivateValueOrACredentialShapeIsRefusedAndAShortPrivateValueBlocksNothing"},
	{"records belong to one step", "TakeCalls keeps the calls it returned", "red: the quiet step carries calls", "TestEachStepRecordsItsRequestsWithMaskedPathsAndItsRefs"},
	{"poll stops at the deadline", "skip the deadline check after a failed attempt", "red: the delta step never finishes or reports another attempt count", "TestPollGivesUpAtTheDeadlineAndReportsTheAttemptCount"},
}

func TestHostAPIMismatchExitsTwoBeforeManifestAndNamesBothValues(t *testing.T) {
	w := newWorld(t, core, nil)
	if err := os.WriteFile(w.env["COCKPIT_DECLARATION"], []byte("hostApi: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := w.run(nil)
	joined := strings.Join(got.stderr, "\n")
	if got.code != 2 || !strings.Contains(joined, "7") || !strings.Contains(joined, "1") || w.exists("manifest.json") || len(w.paths()) != 0 {
		t.Fatalf("exit %d stderr %q manifest=%v", got.code, joined, w.exists("manifest.json"))
	}
	if err := os.WriteFile(w.env["COCKPIT_DECLARATION"], []byte("hostApi: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := w.run(nil); got.code != 0 {
		t.Fatalf("same value: exit %d stderr %v", got.code, got.stderr)
	}
}

func TestMissingOrUnreadableDeclarationExitsTwo(t *testing.T) {
	w := newWorld(t, core, nil)
	os.Remove(w.env["COCKPIT_DECLARATION"])
	if got := w.run(nil); got.code != 2 || w.exists("manifest.json") {
		t.Fatalf("missing: exit %d stderr %v", got.code, got.stderr)
	}
	for _, body := range []string{"hostApi: one\n", "scripts: {}\n", ":::\n"} {
		if err := os.WriteFile(w.env["COCKPIT_DECLARATION"], []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := w.run(nil); got.code != 2 {
			t.Fatalf("%q: exit %d stderr %v", body, got.code, got.stderr)
		}
	}
}

func TestBadArgumentsExitTwoWithReason(t *testing.T) {
	w := newWorld(t, core, nil)
	for _, args := range [][]string{{"beta"}, {"--flow"}, {"--what"}, {"--flow", "Bad Id"}} {
		got := w.run(nil, args...)
		if got.code != 2 || len(got.stderr) == 0 || w.exists("manifest.json") {
			t.Fatalf("%v: exit %d stderr %v", args, got.code, got.stderr)
		}
	}
	var out, errOut strings.Builder
	if code := Main(t.Context(), []string{"Bad/name"}, func(string) string { return "" }, Deps{Stdout: &out, Stderr: &errOut}); code != 2 || !strings.Contains(errOut.String(), "does not match") {
		t.Fatalf("exit %d stderr %q", code, errOut.String())
	}
}

func TestMissingOrBrokenSuiteFileExitsTwoWithoutManifest(t *testing.T) {
	w := newWorld(t, core, nil)
	suite := filepath.Join(w.scripts, "suites", "fake.ts")
	for name, body := range map[string]string{
		"syntax error":        "export default {\n",
		"not a suite":         "export default { name: \"fake\" };\n",
		"name mismatch":       "import { suite } from \"cockpit:flow\";\nexport default suite({ name: \"other\", stages: [], flows: [] });\n",
		"import outside root": "import { x } from \"../../outside.ts\";\nexport default x;\n",
	} {
		if err := os.WriteFile(suite, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		got := w.run(nil)
		if got.code != 2 || len(got.stderr) == 0 || w.exists("manifest.json") {
			t.Fatalf("%s: exit %d stderr %v", name, got.code, got.stderr)
		}
	}
	os.Remove(suite)
	if got := w.run(nil); got.code != 2 {
		t.Fatalf("missing: exit %d", got.code)
	}
}

func TestRunnerMutationRecordMatchesTestFilesBothWays(t *testing.T) {
	porttest.AssertMutationRecordAcross(t, []string{"main_test.go", "execute_test.go", "exec_test.go", "helpers_test.go", "records_test.go"}, runnerMutations)
}
