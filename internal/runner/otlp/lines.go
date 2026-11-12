package otlp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"sync"
	"unicode/utf8"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

const (
	MaxString = 1 << 10
	MaxLine   = 64 << 10
	minChunk  = 64
)

type Writer struct {
	mu   sync.Mutex
	out  io.Writer
	mask *mask.Masker
}

func NewWriter(out io.Writer) (*Writer, error) {
	m, err := mask.New(mask.Options{Mode: mask.Runner})
	if err != nil {
		return nil, err
	}
	return &Writer{out: out, mask: m}, nil
}

func (w *Writer) Write(sigs []Signal) error {
	lines := make([][]byte, len(sigs)+1)
	errs := make([]error, len(sigs))
	chunk := max(minChunk, (len(sigs)+runtime.GOMAXPROCS(0)-1)/runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for from := 0; from < len(sigs); from += chunk {
		wg.Go(func() {
			for i := from; i < min(from+chunk, len(sigs)); i++ {
				lines[i], errs[i] = w.line(sigs[i])
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	buf := bytes.Join(lines, []byte{'\n'})
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.out.Write(buf)
	return err
}

func (w *Writer) line(s Signal) ([]byte, error) {
	s = w.redact(s)
	line, err := json.Marshal(Line{V: LineVersion, Signal: s})
	for _, shrink := range shrinks {
		if err != nil || len(line) < MaxLine {
			break
		}
		shrink(&s)
		line, err = json.Marshal(Line{V: LineVersion, Signal: s})
	}
	return line, err
}

var shrinks = []func(*Signal){
	func(s *Signal) {
		service := s.Attrs[ServiceAttr]
		s.Attrs = contract.Attrs{ServiceAttr: service, "attrsDropped": len(s.Attrs) - 1}
	},
	func(s *Signal) {
		if s.Span != nil {
			s.Span.Links = nil
		}
	},
	func(s *Signal) {
		if s.Metric != nil && s.Metric.Histogram != nil {
			s.Metric.Histogram.Bounds, s.Metric.Histogram.Counts = nil, nil
		}
	},
}

func (w *Writer) redact(s Signal) Signal {
	text := func(v string) string { return cut(w.mask.Mask(v)) }
	s.ID = text(s.ID)
	s.TS = text(s.TS)
	s.Attrs = cutAll(w.mask.Leaves(map[string]any(s.Attrs))).(map[string]any)
	if s.Span != nil {
		sp := *s.Span
		sp.TraceID, sp.ParentID, sp.Name = text(sp.TraceID), text(sp.ParentID), text(sp.Name)
		sp.Start, sp.End = text(sp.Start), text(sp.End)
		links := make([]contract.SpanLink, len(sp.Links))
		for i, l := range sp.Links {
			links[i] = contract.SpanLink{TraceID: text(l.TraceID), SpanID: text(l.SpanID)}
		}
		if len(links) > 0 {
			sp.Links = links
		}
		s.Span = &sp
	}
	if s.Metric != nil {
		m := *s.Metric
		m.Name, m.Unit = text(m.Name), text(m.Unit)
		s.Metric = &m
	}
	return s
}

func cutAll(v any) any {
	switch x := v.(type) {
	case string:
		return cut(x)
	case []any:
		for i, e := range x {
			x[i] = cutAll(e)
		}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = cutAll(e)
		}
		return out
	}
	return v
}

func cut(s string) string {
	if len(s) <= MaxString {
		return s
	}
	n := MaxString
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
