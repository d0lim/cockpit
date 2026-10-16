package totp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/clock"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const rfcSeed = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
const plantedSeed = "PLANTEDSEEDAAAAABBBBBCCCCCDDDDD2"

var totpMutations = []porttest.Mutation{
	{"RFC 6238 codes", "pin the HMAC dynamic truncation offset to 0", "red: the 59-second vector is not 287082", "TestRFC6238VectorsAndBase32DecodingMatch"},
	{"a second issue in the same window waits for the next window", "ChooseStep ignores last", "red: the second issue returns the same step without waiting", "TestSecondIssueInSame30sWindowWaitsForNextWindowAndCommitsStepFirst"},
	{"the step is recorded before the code", "Issue calls CommitStep after making the code", "red: a code goes out even when recording the step fails", "TestSecondIssueInSame30sWindowWaitsForNextWindowAndCommitsStepFirst"},
	{"the seed never appears in error messages", "put the seed value in the unknown account error", "red: the PLANTED seed shows in the error message", "TestUnknownAccountAndEmptySeedAreRejectedWithoutTheSeedInTheMessage"},
	{"in-place save", "SeedFile.Save writes through rename", "red: the inode changes and the mode is not 0600", "TestSeedSaveRewritesPrivateEnvInPlaceAndKeepsOtherSections"},
}

type fakeSteps struct {
	last    map[string]int64
	commits []int64
	fail    bool
}

func (f *fakeSteps) LastStep(id string) (int64, bool) { v, ok := f.last[id]; return v, ok }
func (f *fakeSteps) CommitStep(id string, step int64) error {
	if f.fail {
		return errors.New("commit failed")
	}
	f.last[id] = step
	f.commits = append(f.commits, step)
	return nil
}

func issuer(now *int64, steps *fakeSteps, slept *[]int64) Issuer {
	return Issuer{
		Seeds: func(id string) (string, bool) {
			if id == "admin-1" {
				return rfcSeed, true
			}
			if id == "empty" {
				return "", true
			}
			return "", false
		},
		Clock: clock.Clock{Now: func() time.Time { return time.UnixMilli(*now) }},
		Sleep: func(_ context.Context, ms int64) error { *slept = append(*slept, ms); *now += ms; return nil },
		Steps: steps,
		Mask:  ports.Identity,
	}
}

func TestRFC6238VectorsAndBase32DecodingMatch(t *testing.T) {
	raw, err := DecodeBase32("MZXW6===")
	if err != nil || string(raw) != "foo" {
		t.Fatalf("%q %v", raw, err)
	}
	if _, err := DecodeBase32("MZXW6=1="); err == nil {
		t.Fatal("character outside the alphabet")
	}
	code, err := At(rfcSeed, 1)
	if err != nil || code != "287082" {
		t.Fatalf("%q %v", code, err)
	}
	code, err = At(rfcSeed, 1111111109/30)
	if err != nil || code != "081804" {
		t.Fatalf("%q %v", code, err)
	}
}

func TestSecondIssueInSame30sWindowWaitsForNextWindowAndCommitsStepFirst(t *testing.T) {
	now := int64(90_000 + 100)
	steps := &fakeSteps{last: map[string]int64{}}
	var slept []int64
	iss := issuer(&now, steps, &slept)
	first, err := iss.Issue(context.Background(), "admin-1")
	if err != nil || first == "" || len(slept) != 0 || steps.commits[0] != 3 {
		t.Fatalf("first issue: %q %v slept=%v commits=%v", first, err, slept, steps.commits)
	}
	second, err := iss.Issue(context.Background(), "admin-1")
	if err != nil || second == "" || len(slept) != 0 || steps.commits[1] != 4 {
		t.Fatalf("second issue (the next window is inside the verify window, so no wait): %q %v slept=%v commits=%v", second, err, slept, steps.commits)
	}
	third, err := iss.Issue(context.Background(), "admin-1")
	if err != nil || third == "" || len(slept) != 1 || slept[0] != 4*30_000-(90_000+100)+300 || steps.commits[2] != 5 {
		t.Fatalf("third issue is outside the window, so it waits: %q %v slept=%v commits=%v", third, err, slept, steps.commits)
	}
	steps.fail = true
	if code, err := iss.Issue(context.Background(), "admin-1"); err == nil || code != "" {
		t.Fatalf("no code goes out when recording the step fails: %q %v", code, err)
	}
}

func TestUnknownAccountAndEmptySeedAreRejectedWithoutTheSeedInTheMessage(t *testing.T) {
	now := int64(0)
	steps := &fakeSteps{last: map[string]int64{}}
	var slept []int64
	iss := issuer(&now, steps, &slept)
	iss.Seeds = func(id string) (string, bool) {
		if id == "planted" {
			return plantedSeed, true
		}
		return "", id == "empty"
	}
	var pe *engine.PortError
	_, err := iss.Issue(context.Background(), "nobody")
	if !errors.As(err, &pe) || pe.Code != engine.PortBadArgs || !strings.Contains(pe.Message, "nobody") {
		t.Fatalf("%v", err)
	}
	_, err = iss.Issue(context.Background(), "empty")
	if !errors.As(err, &pe) || pe.Code != engine.PortUnavailable {
		t.Fatalf("%v", err)
	}
	steps.fail = true
	_, err = iss.Issue(context.Background(), "planted")
	if err == nil || strings.Contains(err.Error(), plantedSeed) || strings.Contains(err.Error(), plantedSeed[:8]) {
		t.Fatalf("%v", err)
	}
}

func TestSeedSaveRewritesPrivateEnvInPlaceAndKeepsOtherSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e.private.env.json")
	if err := os.WriteFile(path, []byte(`{"local":{"adminPassword":"PLANTED-pw-123456"},"meta":{"generatedAt":"t0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	local, err := SeedFile{Path: path}.Save("totpSecret1", plantedSeed)
	if err != nil || local["totpSecret1"] != plantedSeed || local["adminPassword"] != "PLANTED-pw-123456" {
		t.Fatalf("%v %v", local, err)
	}
	after, _ := os.Stat(path)
	if after.Mode().Perm() != 0o600 || before.Sys().(*syscall.Stat_t).Ino != after.Sys().(*syscall.Stat_t).Ino {
		t.Fatalf("mode %v inode %v→%v", after.Mode(), before.Sys().(*syscall.Stat_t).Ino, after.Sys().(*syscall.Stat_t).Ino)
	}
	var doc map[string]any
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &doc); err != nil || doc["meta"].(map[string]any)["generatedAt"] != "t0" {
		t.Fatalf("%s %v", raw, err)
	}
	if _, err := (SeedFile{Path: filepath.Join(t.TempDir(), "missing.json")}).Save("k", "v"); err == nil {
		t.Fatal("a missing file must not be created")
	}
	got, err := ReadPrivateEnv(path)
	if err != nil || got["totpSecret1"] != plantedSeed {
		t.Fatalf("%v %v", got, err)
	}
}

func TestTOTPMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "totp_test.go", totpMutations)
}
