package api

import (
	"strconv"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/wire"
)

func (r *Reader) WireCall(id, take string, seq int64) (WireCallView, error) {
	run, err := r.wireRun(id)
	if err != nil {
		return WireCallView{}, err
	}
	if take == "" {
		takes, err := run.store.Takes()
		if err != nil {
			return WireCallView{}, run.mask.problem(err)
		}
		if len(takes) == 0 {
			return WireCallView{}, notFound("this run has no stored take")
		}
		take = defaultTake(takes)
	}
	rec, err := r.wireRecord(run, take)
	if err != nil {
		return WireCallView{}, err
	}
	at, ok := rec.bySeq[seq]
	if !ok {
		return WireCallView{}, notFound("this take has no call " + strconv.FormatInt(seq, 10))
	}
	stored, err := run.store.CallAt(take, rec.index.Heads[at])
	if err != nil {
		return WireCallView{}, run.mask.problem(err)
	}
	fields := make([]mask.PublicField, 0, len(rec.index.PublicFields))
	for _, f := range rec.index.PublicFields {
		fields = append(fields, mask.PublicField(f))
	}
	public, err := run.mask.m.WithPublicFields(fields)
	if err != nil {
		return WireCallView{}, run.mask.problem(err)
	}
	view := WireCallView{ID: run.view.ID, Take: take, Head: wireHead(rec, at, run.mask), Call: wire.Clean(public, stored)}
	view.Flow, view.Step, view.Why = wireWhy(rec, at, view.Call.Span, run.mask)
	return view, nil
}

func wireWhy(rec *wireRecord, at int, span string, m *wireMask) (flow, step, why string) {
	raw := rec.index.Heads[at]
	head := raw
	head.Caller, head.Target = m.text(raw.Caller), m.text(raw.Target)
	named := ""
	if s := raw.Link.Step; raw.Link.Kind != wire.LinkNone && s >= 0 && s < len(rec.index.Steps) {
		flow, step = m.text(rec.index.Steps[s].Flow), m.text(rec.index.Steps[s].Step)
		named = flow + " › " + step
	}
	var parent *wire.Head
	if p, ok := rec.bySeq[raw.Link.Parent]; ok && raw.Link.Kind == wire.LinkNested {
		h := rec.index.Heads[p]
		h.Caller, h.Target = m.text(h.Caller), m.text(h.Target)
		parent = &h
	}
	return flow, step, head.Why(span, named, parent)
}
