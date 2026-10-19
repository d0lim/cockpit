package sse

type JobPhase string

const (
	JobStart JobPhase = "start"
	JobOut   JobPhase = "out"
	JobExit  JobPhase = "exit"
)

type JobEvent struct {
	JobID string   `json:"jobId"`
	Title string   `json:"title"`
	Phase JobPhase `json:"phase"`
	Text  string   `json:"text,omitempty"`
	Code  *int     `json:"code,omitempty"`
}

func JobStarted(id, title, command string) JobEvent {
	return JobEvent{JobID: id, Title: title, Phase: JobStart, Text: command}
}

func JobOutput(id, title, line string) JobEvent {
	return JobEvent{JobID: id, Title: title, Phase: JobOut, Text: line}
}

func JobExited(id, title string, code int) JobEvent {
	return JobEvent{JobID: id, Title: title, Phase: JobExit, Code: &code}
}

func (b *Bus) PublishJob(target string, e JobEvent) Event {
	return b.Publish(KindJob, target, e)
}
