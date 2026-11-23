package archive

import (
	"bytes"
	"encoding/json"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/wire"
)

func (r *reader) proveTakes() error {
	takes := map[string]int{}
	for rel := range r.report {
		if take, ok := takeOf(rel); ok {
			takes[take]++
		}
	}
	for take, n := range takes {
		dir := DirReport + "/" + wire.StagingDir + "/" + take + "/"
		if n != 2 || r.refused(dir) {
			continue
		}
		clean, err := r.proveTake(dir)
		if err != nil {
			return err
		}
		if !clean {
			r.left = append(r.left, r.m.Mask(dir+wire.CallsFile))
			r.root.Remove(dir + wire.CallsFile)
		}
	}
	return nil
}

func (r *reader) refused(dir string) bool {
	for _, name := range r.left {
		if len(name) >= len(dir) && name[:len(dir)] == dir {
			return true
		}
	}
	return false
}

func (r *reader) proveTake(dir string) (bool, error) {
	raw, err := r.root.ReadFile(dir + wire.IndexFile)
	if err != nil {
		return false, err
	}
	var index wire.Index
	if err := json.Unmarshal(raw, &index); err != nil {
		return false, &Invalid{Reason: "a wire index does not read as one", Files: []string{r.m.Mask(dir + wire.IndexFile)}, cause: err}
	}
	fields := make([]mask.PublicField, 0, len(index.PublicFields))
	for _, f := range index.PublicFields {
		fields = append(fields, mask.PublicField(f))
	}
	public, err := r.m.WithPublicFields(fields)
	if err != nil {
		return false, &Invalid{Reason: "a wire take declares a public field the hub refuses", Files: []string{r.m.Mask(dir + wire.IndexFile)}}
	}
	calls, err := r.root.ReadFile(dir + wire.CallsFile)
	if err != nil {
		return false, err
	}
	lines := bytes.Split(bytes.TrimSuffix(calls, []byte("\n")), []byte("\n"))
	if len(calls) == 0 {
		lines = nil
	}
	if len(lines) != len(index.Heads) {
		return false, r.invalid("a wire index does not point at the lines of its calls", dir+wire.IndexFile)
	}
	var offset int64
	for i, line := range lines {
		var l wire.Line
		if err := json.Unmarshal(line, &l); err != nil || l.Call == nil {
			return false, &Invalid{Reason: "a wire call does not read as one", Files: []string{r.m.Mask(dir + wire.CallsFile)}}
		}
		head := index.Heads[i]
		if head.Offset != offset || head.Length != int64(len(line)) || head.Seq != l.Call.Seq {
			return false, r.invalid("a wire index does not point at the lines of its calls", dir+wire.IndexFile)
		}
		offset += int64(len(line)) + 1
		before, err := json.Marshal(l)
		if err != nil {
			return false, err
		}
		clean, _ := wire.Prove(public, *l.Call)
		after, err := json.Marshal(wire.Line{Call: &clean})
		if err != nil || !bytes.Equal(before, after) {
			return false, err
		}
	}
	return true, nil
}
