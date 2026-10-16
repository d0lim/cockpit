package runner

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"

	"github.com/d0lim/cockpit/internal/contract"
)

var flowIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,40}$`)

type StepDecl struct {
	ID     string                   `json:"id"`
	Title  string                   `json:"title"`
	Kind   contract.StepKind        `json:"kind"`
	Source *contract.ManifestSource `json:"source,omitempty"`
}

type FlowDecl struct {
	ID       string                   `json:"id"`
	Title    string                   `json:"title"`
	Stage    string                   `json:"stage"`
	Once     bool                     `json:"once"`
	Requires []string                 `json:"requires"`
	Steps    []StepDecl               `json:"steps"`
	Source   *contract.ManifestSource `json:"source,omitempty"`
}

type SuiteDecl struct {
	Name   string                   `json:"name"`
	Stages []contract.ManifestStage `json:"stages"`
	Flows  []FlowDecl               `json:"flows"`
}

func Order(s SuiteDecl) ([]FlowDecl, error) {
	stageIndex := map[string]int{}
	for i, st := range s.Stages {
		stageIndex[st.ID] = i
	}
	seen := map[string]bool{}
	for _, f := range s.Flows {
		if !flowIDPattern.MatchString(f.ID) {
			return nil, fmt.Errorf("플로우 id %q 가 %s 꼴이 아니다", f.ID, flowIDPattern)
		}
		if seen[f.ID] {
			return nil, fmt.Errorf("플로우 id %q 가 겹친다", f.ID)
		}
		seen[f.ID] = true
		if _, ok := stageIndex[f.Stage]; !ok {
			return nil, fmt.Errorf("플로우 %s 의 스테이지 %q 가 스위트에 없다", f.ID, f.Stage)
		}
		if err := checkSteps(f); err != nil {
			return nil, err
		}
	}
	ordered := slices.Clone(s.Flows)
	slices.SortStableFunc(ordered, func(a, b FlowDecl) int { return cmp.Compare(stageIndex[a.Stage], stageIndex[b.Stage]) })
	before := map[string]bool{}
	for _, f := range ordered {
		for _, r := range f.Requires {
			if !seen[r] {
				return nil, fmt.Errorf("플로우 %s 의 전제 %q 가 스위트에 없다", f.ID, r)
			}
			if !before[r] {
				return nil, fmt.Errorf("플로우 %s 의 전제 %q 가 그보다 뒤에서 돈다", f.ID, r)
			}
		}
		before[f.ID] = true
	}
	return ordered, nil
}

func checkSteps(f FlowDecl) error {
	seen := map[string]bool{}
	for _, s := range f.Steps {
		if s.ID == "" {
			return fmt.Errorf("플로우 %s 에 id 없는 단계가 있다", f.ID)
		}
		if seen[s.ID] {
			return fmt.Errorf("플로우 %s 에 단계 id %q 가 겹친다", f.ID, s.ID)
		}
		seen[s.ID] = true
		switch s.Kind {
		case contract.StepKindRequest, contract.StepKindWait, contract.StepKindCapture:
		default:
			return fmt.Errorf("단계 %s 의 종류 %q 가 request·wait·capture 가운데 하나가 아니다", s.ID, s.Kind)
		}
	}
	return nil
}

func Manifest(s SuiteDecl, ordered []FlowDecl, identity contract.RunnerIdentity) contract.Manifest {
	m := contract.Manifest{
		SchemaVersion: contract.RunnerSchemaVersion,
		Suite:         s.Name,
		Stages:        append([]contract.ManifestStage{}, s.Stages...),
		Flows:         []contract.ManifestFlow{},
		Runner:        &identity,
	}
	for _, f := range ordered {
		mf := contract.ManifestFlow{ID: f.ID, Title: f.Title, Stage: f.Stage, Once: f.Once, Requires: append([]string{}, f.Requires...), Steps: []contract.ManifestStep{}, Source: f.Source}
		for _, st := range f.Steps {
			mf.Steps = append(mf.Steps, contract.ManifestStep{ID: st.ID, Title: st.Title, Kind: st.Kind, Source: st.Source})
		}
		m.Flows = append(m.Flows, mf)
	}
	return m
}
