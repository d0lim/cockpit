package wire

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

func Export(m *mask.Masker, index Index, calls io.ReaderAt, out io.Writer) (Index, error) {
	fields := make([]mask.PublicField, 0, len(index.PublicFields))
	for _, f := range index.PublicFields {
		fields = append(fields, mask.PublicField(f))
	}
	public, err := m.WithPublicFields(fields)
	if err != nil {
		return Index{}, err
	}
	exported := Index{Status: index.Status, Steps: make([]StepRef, len(index.Steps)), Heads: make([]Head, 0, len(index.Heads))}
	exported.Status.Detail = m.Mask(index.Status.Detail)
	exported.Status.WithheldCalls = 0
	for i, s := range index.Steps {
		exported.Steps[i] = StepRef{Flow: m.Mask(s.Flow), Step: m.Mask(s.Step), Status: s.Status}
	}
	for _, f := range index.PublicFields {
		exported.PublicFields = append(exported.PublicFields, contract.WirePublicField{Key: m.Mask(f.Key), Value: m.Mask(f.Value)})
	}
	for _, r := range index.Routes {
		r.Name, r.Service = contract.Name(m.Mask(string(r.Name))), m.Mask(r.Service)
		exported.Routes = append(exported.Routes, r)
	}
	var offset int64
	for _, head := range index.Heads {
		stored, err := storedCall(calls, head)
		if err != nil {
			return Index{}, err
		}
		clean, _ := Prove(public, stored)
		encoded, err := json.Marshal(Line{Call: &clean})
		if err != nil {
			return Index{}, err
		}
		if _, err := out.Write(append(encoded, '\n')); err != nil {
			return Index{}, err
		}
		leaving := headOf(clean, offset, int64(len(encoded)))
		leaving.Link = head.Link
		leaving.Link.Root = m.Mask(head.Link.Root)
		if leaving.Withheld {
			leaving.Link.Root = ""
			exported.Status.WithheldCalls++
		}
		exported.Heads = append(exported.Heads, leaving)
		offset += int64(len(encoded)) + 1
	}
	return exported, nil
}

func storedCall(calls io.ReaderAt, head Head) (Call, error) {
	if head.Length <= 0 || head.Length > maxTakeBytes {
		return Call{}, fmt.Errorf("call %d has no readable line in its index", head.Seq)
	}
	text := make([]byte, head.Length)
	if _, err := calls.ReadAt(text, head.Offset); err != nil {
		return Call{}, fmt.Errorf("call %d cannot be read: %w", head.Seq, err)
	}
	var line Line
	if err := json.Unmarshal(text, &line); err != nil || line.Call == nil || line.Call.Seq != head.Seq {
		return Call{}, fmt.Errorf("call %d does not match its index", head.Seq)
	}
	return *line.Call, nil
}
