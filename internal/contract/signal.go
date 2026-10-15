package contract

type SignalKind string

const (
	SignalKindMetric SignalKind = "metric"
	SignalKindEvent  SignalKind = "event"
	SignalKindSpan   SignalKind = "span"
)

type TimeSource string

const (
	TimeSourceHost      TimeSource = "host"
	TimeSourceContainer TimeSource = "container"
	TimeSourceRunner    TimeSource = "runner"
)

type Tone string

const (
	ToneOK   Tone = "ok"
	ToneWarn Tone = "warn"
	ToneBad  Tone = "bad"
	ToneIdle Tone = "idle"
	ToneInfo Tone = "info"
)

type SpanStatus string

const (
	SpanStatusOK         SpanStatus = "ok"
	SpanStatusError      SpanStatus = "error"
	SpanStatusIncomplete SpanStatus = "incomplete"
)

type ReservedAttrs struct {
	Table  string `json:"table,omitempty"`
	Status string `json:"status,omitempty"`
	Label  string `json:"label,omitempty"`
	Tone   Tone   `json:"tone,omitempty"`
}

type Attrs map[string]any

type SignalInput struct {
	TS       string     `json:"ts,omitempty"`
	TSSource TimeSource `json:"tsSource,omitempty"`
	ID       string     `json:"id,omitempty"`
	Rev      int        `json:"rev,omitempty"`
	Attrs    Attrs      `json:"attrs,omitempty"`
}

type MetricBody struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type EventBody struct {
	Body string `json:"body,omitempty"`
}

type SpanLink struct {
	TraceID string `json:"traceId"`
	SpanID  string `json:"spanId"`
}

type SpanBody struct {
	TraceID  string     `json:"traceId"`
	ParentID string     `json:"parentId,omitempty"`
	Name     string     `json:"name"`
	Start    string     `json:"start"`
	End      string     `json:"end,omitempty"`
	Status   SpanStatus `json:"status"`
	Links    []SpanLink `json:"links,omitempty"`
}

type Signal struct {
	Seq       int64       `json:"seq"`
	TS        string      `json:"ts"`
	TSSource  TimeSource  `json:"tsSource"`
	Kind      SignalKind  `json:"kind"`
	Collector string      `json:"collector"`
	ID        string      `json:"id,omitempty"`
	Rev       int         `json:"rev,omitempty"`
	Attrs     Attrs       `json:"attrs"`
	Metric    *MetricBody `json:"metric,omitempty"`
	Event     *EventBody  `json:"event,omitempty"`
	Span      *SpanBody   `json:"span,omitempty"`
}
