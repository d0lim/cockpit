package observer

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports"
	dockerport "github.com/d0lim/cockpit/internal/ports/docker"
	"github.com/d0lim/cockpit/internal/signal"
)

const (
	OTLPMaxLine  = 64 << 10
	DropUnparsed = "unparsed"
	DropRefused  = "refused"
	otlpVersion  = 1
	stateRunning = "running"
)

type OTLPStream interface {
	Emitter
	KVGet(collector string) (map[string]any, error)
	KVPut(collector string, value map[string]any) error
}

type RoleLister interface {
	RoleContainers(ctx context.Context, role string) ([]dockerport.Container, error)
}

type OTLP struct {
	RunID     string
	Lister    RoleLister
	Docker    compose.Docker
	Stream    OTLPStream
	Mask      ports.Mask
	Log       *slog.Logger
	Poll      time.Duration
	SaveEvery time.Duration
	Now       func() time.Time

	once   sync.Once
	mu     sync.Mutex
	traces map[string]string
	state  otlpState
	floor  time.Time
	saved  time.Time
	heard  atomic.Int64
}

type otlpLine struct {
	V      int              `json:"v"`
	Signal *contract.Signal `json:"signal"`
}

func (o *OTLP) Name() string { return signal.OTLPCollector }

func (o *OTLP) defaults() {
	o.once.Do(func() {
		if o.Mask == nil {
			o.Mask = ports.Identity
		}
		if o.Log == nil {
			o.Log = slog.New(slog.DiscardHandler)
		}
		if o.Poll <= 0 {
			o.Poll = DefaultPoll
		}
		if o.SaveEvery <= 0 {
			o.SaveEvery = time.Second
		}
		if o.Now == nil {
			o.Now = time.Now
		}
		o.mu.Lock()
		o.registerTrace(runTraceID(o.RunID), o.RunID)
		o.mu.Unlock()
	})
}

func (o *OTLP) Run(ctx context.Context) error {
	o.defaults()
	o.load()
	defer o.save()
	followed := map[string]bool{}
	for {
		cs, err := o.Lister.RoleContainers(ctx, compose.RoleOTLP)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		for _, c := range cs {
			key := c.ID + "/" + c.State
			if c.State != stateRunning && followed[key] {
				continue
			}
			followed[key] = true
			o.follow(ctx, c.ID)
			if ctx.Err() == nil && c.State == stateRunning {
				o.gap()
			}
		}
		t := time.NewTimer(o.Poll)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil
		case <-t.C:
		}
	}
}

func (o *OTLP) follow(ctx context.Context, id string) {
	o.mu.Lock()
	from := o.state.cursor
	o.floor = from
	o.mu.Unlock()
	args := []string{"logs", "--follow", "--timestamps"}
	if !from.IsZero() {
		args = append(args, "--since="+from.Format(time.RFC3339Nano))
	}
	out := &lineWriter{emit: o.line, max: OTLPMaxLine}
	_, _ = o.Docker.Run(ctx, compose.Command{Args: append(args, id), Stdout: out, Stderr: io.Discard})
	out.flush()
	o.save()
}

func (o *OTLP) line(raw string) {
	stamp, text, ok := strings.Cut(raw, " ")
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if !ok || err != nil {
		at, text = o.Now().UTC(), raw
	}
	o.mu.Lock()
	if !o.floor.IsZero() && !at.After(o.floor) {
		o.mu.Unlock()
		return
	}
	if at.After(o.state.cursor) {
		o.state.cursor = at
	}
	o.mu.Unlock()
	sig, ok := o.parse(text)
	switch {
	case !ok:
		o.count(DropUnparsed)
	default:
		if sig.Span != nil {
			o.heard.Store(time.Now().UnixNano())
		}
		if _, err := o.Stream.Append(sig); err != nil {
			o.count(DropRefused)
			o.Log.Warn("an OTLP signal was refused", "run", o.RunID, "err", o.Mask(err.Error()))
		}
	}
	o.saveIfDue()
}

func (o *OTLP) parse(text string) (contract.Signal, bool) {
	if len(text) > OTLPMaxLine {
		return contract.Signal{}, false
	}
	var l otlpLine
	if err := json.Unmarshal([]byte(text), &l); err != nil || l.V != otlpVersion || l.Signal == nil {
		return contract.Signal{}, false
	}
	sig := *l.Signal
	sig.Seq, sig.Collector = 0, signal.OTLPCollector
	if sig.Span != nil {
		span := *sig.Span
		span.TraceID = o.traceFor(span.TraceID)
		sig.Span = &span
	}
	return sig, true
}

func (o *OTLP) gap() {
	o.mu.Lock()
	from := "its first line"
	attrs := contract.Attrs{"status": "gap", "tone": string(contract.ToneWarn)}
	if !o.state.cursor.IsZero() {
		from = o.state.cursor.UTC().Format(time.RFC3339Nano)
		attrs["from"] = from
	}
	o.mu.Unlock()
	_, err := o.Stream.Append(contract.Signal{
		Kind: contract.SignalKindEvent, Collector: signal.OTLPCollector, TSSource: contract.TimeSourceHost, Attrs: attrs,
		Event: &contract.EventBody{Body: "the otlp receiver output ended after " + from + "; the hub follows it again from there once the receiver runs"},
	})
	if err != nil {
		o.Log.Warn("cannot record the end of the otlp receiver output", "run", o.RunID, "err", o.Mask(err.Error()))
	}
}
