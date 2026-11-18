package wire

import (
	"bytes"
	"encoding/json"
	"time"
)

type BodyKind string

const (
	BodyJSON    BodyKind = "json"
	BodyForm    BodyKind = "form"
	BodyText    BodyKind = "text"
	BodyJWT     BodyKind = "jwt"
	BodyOmitted BodyKind = "omitted"
)

type OmitReason string

const (
	OmitTooLarge   OmitReason = "too-large"
	OmitBinary     OmitReason = "binary"
	OmitEncoded    OmitReason = "encoded"
	OmitBusy       OmitReason = "busy"
	OmitRecordFull OmitReason = "record-full"
)

type Outcome string

const (
	OutcomeAnswered   Outcome = "answered"
	OutcomeNoUpstream Outcome = "no-upstream"
	OutcomeCallerGone Outcome = "caller-gone"
	OutcomeIncomplete Outcome = "incomplete"
)

type JWT struct {
	Header map[string]any `json:"header"`
	Claims map[string]any `json:"claims"`
}

type Body struct {
	Kind    BodyKind            `json:"kind,omitempty"`
	JSON    any                 `json:"json,omitempty"`
	Form    map[string][]string `json:"form,omitempty"`
	Text    string              `json:"text,omitempty"`
	JWT     *JWT                `json:"jwt,omitempty"`
	Omitted OmitReason          `json:"omitted,omitempty"`
}

func (b *Body) UnmarshalJSON(raw []byte) error {
	type plain Body
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return dec.Decode((*plain)(b))
}

type Message struct {
	Headers map[string][]string `json:"headers,omitempty"`
	Size    int64               `json:"size"`
	Body    Body                `json:"body,omitzero"`
}

type Call struct {
	Seq      int64     `json:"seq"`
	Caller   string    `json:"caller"`
	Target   string    `json:"target"`
	Port     int       `json:"port"`
	Method   string    `json:"method"`
	Path     string    `json:"path,omitempty"`
	Query    Body      `json:"query,omitzero"`
	Started  time.Time `json:"started"`
	Replied  time.Time `json:"replied,omitzero"`
	Finished time.Time `json:"finished,omitzero"`
	Status   int       `json:"status"`
	Outcome  Outcome   `json:"outcome"`
	Span     string    `json:"span,omitempty"`
	Withheld bool      `json:"withheld,omitempty"`
	Request  Message   `json:"request,omitzero"`
	Response Message   `json:"response,omitzero"`
}

type End struct {
	Calls         int       `json:"calls"`
	BodiesOmitted int       `json:"bodiesOmitted"`
	CallsDropped  int       `json:"callsDropped"`
	Since         time.Time `json:"since,omitzero"`
}

type Line struct {
	Call *Call `json:"call,omitempty"`
	End  *End  `json:"end,omitempty"`
}
