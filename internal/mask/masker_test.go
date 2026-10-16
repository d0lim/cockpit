package mask

import (
	"fmt"
	"strings"
	"testing"
)

const planted = "planted-cred-7f3a9c"

type scenario struct {
	name string
	run  func(engine) []string
}

var scenarios = []scenario{
	{"masking before truncating leaves no fragment of a planted value at the cut while truncating first does", summarizeOrderScenario},
	{"hub mode skips values under 8 characters and runner mode masks from 6 characters without a visible tail", modeBoundsScenario},
	{"a declared pattern that is not valid RE2 fails masker construction and names its index", invalidPatternScenario},
	{"a masker with a new value list keeps the mode and declared patterns and the original keeps its value list", withKnownScenario},
	{"values from generated env outputs outside the declared sources are masked and public names stay visible", outputsAndPublicScenario},
	{"empty values and ${VAR} literals are not registered and are reported missing with source and name", missingScenario},
	{"a value source that cannot be read or parsed returns only an error and no partial list", refreshFailureScenario},
	{"rescan reports unreadable files and files with values left and counts the files it scanned", rescanUnreadableScenario},
	{"rescan does not follow symlinks, reports links as left and skips the inside of linked directories", rescanLinkScenario},
	{"rescan returns an error when it cannot open the directory", rescanMissingDirScenario},
	{"planted values in string leaves of a structured signal are masked while structure and numbers stay", leavesPlantedScenario},
	{"arrays under credential-named keys have every element masked and arrays under other keys stay", leavesArrayScenario},
	{"numbers and booleans under credential-named keys are masked and numbers under other keys stay", leavesScalarScenario},
	{"values of unknown types go through JSON before the leaf and key rules apply", leavesUnknownTypeScenario},
}

func TestMaskerScenariosAllPassOnTheRealEngine(t *testing.T) {
	ran := 0
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			ran++
			for _, f := range s.run(realEngine) {
				t.Error(f)
			}
		})
	}
	if ran == 0 || ran != len(scenarios) {
		t.Fatalf("ran %d scenarios, registered %d", ran, len(scenarios))
	}
}

func expectEqual(label, got, want string) []string {
	if got != want {
		return []string{fmt.Sprintf("%s: got %q, want %q", label, got, want)}
	}
	return nil
}

func truncate(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

func summarizeOrderScenario(e engine) []string {
	var f []string
	text := strings.Repeat("가", 10) + planted + " trailing text"
	fragment := planted[:5]
	for _, mode := range []Mode{Hub, Runner} {
		m, err := e.build(Options{Mode: mode, Known: []string{planted}})
		if err != nil {
			return []string{err.Error()}
		}
		got := e.summarize(m, text, 15)
		if strings.Contains(got, fragment) || !strings.Contains(got, marker) {
			f = append(f, fmt.Sprintf("mode %d: summary %q keeps a raw fragment or lacks the marker", mode, got))
		}
		if cut := m.Mask(truncate(text, 15)); !strings.Contains(cut, fragment) {
			f = append(f, fmt.Sprintf("mode %d: truncate-then-mask control %q lacks the fragment", mode, cut))
		}
	}
	return f
}

func modeBoundsScenario(e engine) []string {
	hub, err := e.build(Options{Mode: Hub, Known: []string{"abc1234", "abcd1234"}})
	if err != nil {
		return []string{err.Error()}
	}
	runner, err := e.build(Options{Mode: Runner, Known: []string{"ab123", "ab1234"}})
	if err != nil {
		return []string{err.Error()}
	}
	return append(append(append(
		expectEqual("hub 7 chars", e.mask(hub, "x abc1234 y"), "x abc1234 y"),
		expectEqual("hub 8 chars", e.mask(hub, "x abcd1234 y"), "x ****1234 y")...),
		expectEqual("runner 5 chars", e.mask(runner, "x ab123 y"), "x ab123 y")...),
		expectEqual("runner 6 chars", e.mask(runner, "x ab1234 y"), "x **** y")...)
}

func invalidPatternScenario(e engine) []string {
	_, err := e.build(Options{Mode: Hub, Patterns: []string{`ok_[a-z]{8,}`, `(?<![A-Za-z])sk_`}})
	if err == nil || !strings.Contains(err.Error(), "declared pattern 1 ") {
		return []string{fmt.Sprintf("error %v does not point at the second pattern", err)}
	}
	return nil
}

func withKnownScenario(e engine) []string {
	base, err := e.build(Options{Mode: Hub, Patterns: []string{`(?:^|[^A-Za-z])pk_(?P<secret>[A-Za-z0-9]{8,})`}})
	if err != nil {
		return []string{err.Error()}
	}
	next := e.withKnown(base, []string{planted})
	line := "v " + planted + " k pk_abcdefgh12"
	return append(
		expectEqual("new masker", e.mask(next, line), "v ****3a9c k pk_****gh12"),
		expectEqual("original masker", e.mask(base, line), "v "+planted+" k pk_****gh12")...)
}
