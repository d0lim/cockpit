package seal

import (
	"encoding/json"
	"os"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
)

func Verdict(results []Result) contract.SealVerdict {
	if len(results) == 0 {
		return contract.SealError
	}
	inconclusive := false
	for _, r := range results {
		if !r.OK && !r.Inconclusive {
			return contract.SealFailed
		}
		inconclusive = inconclusive || r.Inconclusive
	}
	if inconclusive {
		return contract.SealError
	}
	return contract.SealPassed
}

type Report struct {
	Static []Result
	Probes []Result
}

type entry struct {
	Name         string `json:"name"`
	OK           bool   `json:"ok"`
	Inconclusive bool   `json:"inconclusive,omitempty"`
	Command      string `json:"command,omitempty"`
	Expected     string `json:"expected"`
	Actual       string `json:"actual"`
}

type file struct {
	CheckedAt string               `json:"checkedAt"`
	Verdict   contract.SealVerdict `json:"verdict"`
	OK        bool                 `json:"ok"`
	Static    []entry              `json:"static"`
	Probes    []entry              `json:"probes"`
}

const FileName = "seal.json"

func entries(results []Result, mask func(string) string) []entry {
	out := make([]entry, len(results))
	for i, r := range results {
		out[i] = entry{Name: r.Name, OK: r.OK, Inconclusive: r.Inconclusive, Command: mask(r.Command), Expected: r.Expected, Actual: mask(r.Detail)}
	}
	return out
}

func Encode(report Report, now time.Time, mask func(string) string) ([]byte, error) {
	all := append(append([]Result{}, report.Static...), report.Probes...)
	ok := len(all) > 0
	for _, r := range all {
		ok = ok && r.OK
	}
	body := file{
		CheckedAt: now.UTC().Format(time.RFC3339Nano),
		Verdict:   Verdict(all),
		OK:        ok,
		Static:    entries(report.Static, mask),
		Probes:    entries(report.Probes, mask),
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func Write(root *os.Root, report Report, now time.Time, mask func(string) string) error {
	data, err := Encode(report, now, mask)
	if err != nil {
		return err
	}
	f, err := root.OpenFile(FileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
