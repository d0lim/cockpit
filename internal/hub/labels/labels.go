package labels

//go:generate go run ./gen -out ../../../web/shared/labels.json

import (
	"encoding/json"
	"maps"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

type Table struct {
	States     map[string]string  `json:"states"`
	Stages     map[string]string  `json:"stages"`
	Wait       map[string]string  `json:"wait"`
	Stop       map[string]string  `json:"stop"`
	Destroy    map[string]string  `json:"destroy"`
	Verdicts   map[string]string  `json:"verdicts"`
	Collectors map[string]string  `json:"collectors"`
	FlowStatus map[string]string  `json:"flowStatus"`
	Messages   map[Message]string `json:"messages"`
}

type Message string

const (
	MsgNoRuns           Message = "noRuns"
	MsgListFooter       Message = "listFooter"
	MsgDockerDown       Message = "dockerDown"
	MsgOrphans          Message = "orphans"
	MsgTickError        Message = "tickError"
	MsgKeep             Message = "keep"
	MsgQueued           Message = "queued"
	MsgSubmitted        Message = "submitted"
	MsgMerged           Message = "merged"
	MsgWaitHint         Message = "waitHint"
	MsgWaitResume       Message = "waitResume"
	MsgVerdictLine      Message = "verdictLine"
	MsgNoVerdict        Message = "noVerdict"
	MsgReason           Message = "reason"
	MsgReportHint       Message = "reportHint"
	MsgPoolAllKept      Message = "poolAllKept"
	MsgTimeout          Message = "timeout"
	MsgHubUnreachable   Message = "hubUnreachable"
	MsgHubStart         Message = "hubStart"
	MsgHubWaitAgain     Message = "hubWaitAgain"
	MsgHubLost          Message = "hubLost"
	MsgCandidates       Message = "candidates"
	MsgNowState         Message = "nowState"
	MsgReportPending    Message = "reportPending"
	MsgRedactionFailed  Message = "redactionFailed"
	MsgMetaOnly         Message = "metaOnly"
	MsgStopPoint        Message = "stopPoint"
	MsgFlowResults      Message = "flowResults"
	MsgNotProven        Message = "notProven"
	MsgBundle           Message = "bundle"
	MsgAllowedEvents    Message = "allowedEvents"
	MsgKit              Message = "kit"
	MsgExecCommand      Message = "execCommand"
	MsgExecFlows        Message = "execFlows"
	MsgExecFlowsUnknown Message = "execFlowsUnknown"
	MsgExecExit         Message = "execExit"
	MsgHostAPINotice    Message = "hostApiNotice"
	MsgUnexpected       Message = "unexpected"
)

var messages = map[Message]string{
	MsgNoRuns:           "no runs",
	MsgListFooter:       "kept {pool}/{poolMax} · up {up}/{upCap}",
	MsgDockerDown:       "Docker does not respond",
	MsgOrphans:          "orphans {count}",
	MsgTickError:        "tick error: {error}",
	MsgKeep:             "keep",
	MsgQueued:           "queued #{position}",
	MsgSubmitted:        "run {id}: submitted, queue position {position}",
	MsgMerged:           "run {id}: merged into the queued run of the same commit and suite, queue position {position}",
	MsgWaitHint:         "to wait: {cmd} wait {id}",
	MsgWaitResume:       "if the wait is cut: {cmd} wait {id}",
	MsgVerdictLine:      "{id} · {phase} · verdict {verdict}",
	MsgNoVerdict:        "none",
	MsgReason:           "reason: {reason}",
	MsgReportHint:       "report: {cmd} report {id}",
	MsgPoolAllKept:      "{line}: a person has to act. Every environment in the kept pool has keep on; release one to start: {cmd} unkeep <run id>",
	MsgTimeout:          "timed out; the run keeps going{now}. To wait again: {cmd} wait {id}",
	MsgHubUnreachable:   "the hub does not respond: {url}",
	MsgHubStart:         "to start it: {start}",
	MsgHubWaitAgain:     "after the hub is up, wait again: {cmd} wait {id}",
	MsgHubLost:          "lost the hub connection; reconnecting",
	MsgCandidates:       "runs to stop first (oldest last up first):",
	MsgNowState:         "current state: {phase}",
	MsgReportPending:    "the report is written when the run finishes",
	MsgRedactionFailed:  "the bundle was discarded by the redaction rescan; files:",
	MsgMetaOnly:         "finished without a bundle · stopped at stage {stage}",
	MsgStopPoint:        "stopped at: {where}",
	MsgFlowResults:      "flow results:",
	MsgNotProven:        "not proven: {item}",
	MsgBundle:           "bundle: {dir}",
	MsgAllowedEvents:    "actions: {events}",
	MsgKit:              "kit: {dir}",
	MsgExecCommand:      "exec: {command}",
	MsgExecFlows:        "flows that can run again: {flows}",
	MsgExecFlowsUnknown: "flows that can run again: the manifest has not been read yet",
	MsgExecExit:         "runner exit code {code}",
	MsgHostAPINotice:    "notice: {notice}",
	MsgUnexpected:       "unexpected error: {error}",
}

var verdicts = map[string]string{
	string(lifecycle.VerdictPassed):  "Passed",
	string(lifecycle.VerdictFailed):  "Failed",
	string(lifecycle.VerdictError):   "Error",
	string(lifecycle.VerdictAborted): "Aborted",
}

const FlowNotProven = "notProven"

var flowStatus = map[string]string{
	string(contract.StepStatusNotRun):      "Not run",
	string(contract.StepStatusRunning):     "Running",
	string(contract.StepStatusPassed):      "Passed",
	string(contract.StepStatusFailed):      "Failed",
	string(contract.StepStatusSkipped):     "Skipped",
	string(contract.FlowOutcomeIncomplete): "Incomplete",
	FlowNotProven:                          "Not proven",
}

var collectors = map[string]string{
	"armed":   "Armed",
	"stopped": "Stopped",
	"backoff": "Backing off",
}

func stringKeys[K ~string](in map[K]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[string(k)] = v
	}
	return out
}

func Load() Table {
	return Table{
		States:     stringKeys(lifecycle.StateLabels()),
		Stages:     stringKeys(lifecycle.StageLabels()),
		Wait:       stringKeys(lifecycle.WaitLabels),
		Stop:       stringKeys(lifecycle.StopLabels),
		Destroy:    stringKeys(lifecycle.DestroyLabels),
		Verdicts:   maps.Clone(verdicts),
		Collectors: maps.Clone(collectors),
		FlowStatus: maps.Clone(flowStatus),
		Messages:   maps.Clone(messages),
	}
}

func JSON() ([]byte, error) {
	data, err := json.MarshalIndent(Load(), "", "  ")
	return append(data, '\n'), err
}

func Verdict(v lifecycle.Verdict) *string {
	if v == lifecycle.VerdictNone {
		return nil
	}
	label, ok := verdicts[string(v)]
	if !ok {
		label = string(v)
	}
	return &label
}

func Text(m Message, args map[string]string) string {
	pairs := make([]string, 0, 2*len(args))
	for k, v := range args {
		pairs = append(pairs, "{"+k+"}", v)
	}
	return strings.NewReplacer(pairs...).Replace(messages[m])
}
