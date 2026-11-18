package wire

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

const (
	maxTakeBytes = 256 << 20
	takeAttempts = 2

	detailGone      = "the wire recorder was gone when the take was asked for"
	DetailRestarted = "the wire recorder restarted after the first step, so its record is not whole"
	detailWithheld  = "the value sources of the run could not be read"
	detailBroken    = "the take stream broke off twice"
	detailUnstored  = "the take could not be stored"
	detailName      = "the take name is not valid"
)

var ErrRecorderGone = errors.New("the wire recorder cannot be reached")

type WithheldError struct {
	Detail string
}

func (e *WithheldError) Error() string { return e.Detail }

type Stream func(ctx context.Context, out io.Writer) error

type TakeInput struct {
	Store        Store
	Name         string
	Record       func() (*mask.Masker, error)
	Stream       Stream
	Result       *contract.Result
	PublicFields []contract.WirePublicField
	Routes       []contract.WireRoute
}

func Take(ctx context.Context, in TakeInput) Status {
	if !ValidTake(in.Name) {
		return Status{Take: in.Name, State: StateFailed, Detail: detailName}
	}
	if status, ok := in.Store.stored(in.Name); ok {
		return status
	}
	record, err := in.Record()
	if err != nil {
		detail := detailWithheld
		var why *WithheldError
		if errors.As(err, &why) {
			detail = why.Detail
		}
		return Status{Take: in.Name, State: StateWithheld, Detail: detail}
	}
	w := taking{in: in, record: record, root: in.Store.Root()}
	defer os.RemoveAll(w.partial())
	for attempt := 1; ; attempt++ {
		status, broken := w.attempt(ctx)
		if !broken {
			return status
		}
		if attempt == takeAttempts || ctx.Err() != nil {
			return Status{Take: in.Name, State: StateFailed, Detail: detailBroken}
		}
	}
}

type taking struct {
	in     TakeInput
	record *mask.Masker
	root   string
}

func (w taking) partial() string { return filepath.Join(w.root, partialPrefix+w.in.Name) }

func (w taking) unstored() (Status, bool) {
	return Status{Take: w.in.Name, State: StateFailed, Detail: detailUnstored}, false
}

func (w taking) attempt(ctx context.Context) (status Status, broken bool) {
	if os.RemoveAll(w.partial()) != nil || os.MkdirAll(w.partial(), dirMode) != nil {
		return w.unstored()
	}
	file, err := os.OpenFile(filepath.Join(w.partial(), CallsFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return w.unstored()
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	ended := make(chan error, 1)
	go func() {
		err := w.in.Stream(streamCtx, writer)
		writer.CloseWithError(err)
		ended <- err
	}()
	got := w.copyCalls(reader, file)
	closeErr := file.Close()
	reader.Close()
	cancel()
	streamErr := <-ended
	switch {
	case got.failed || closeErr != nil:
		return w.unstored()
	case errors.Is(streamErr, ErrRecorderGone) && got.bytes == 0:
		return Status{Take: w.in.Name, State: StateLost, Detail: detailGone}, false
	case streamErr != nil || !got.complete():
		return Status{}, true
	case w.restarted(got.end.Since):
		return Status{Take: w.in.Name, State: StateLost, Detail: DetailRestarted}, false
	}
	return w.store(got)
}

type copied struct {
	bytes    int64
	offset   int64
	heads    []Head
	skeleton []Call
	withheld int
	end      *End
	invalid  bool
	failed   bool
}

func (c copied) complete() bool {
	return !c.invalid && c.end != nil && c.end.Calls == len(c.heads)
}

func (w taking) copyCalls(from io.Reader, to io.Writer) copied {
	var got copied
	in := bufio.NewReader(io.LimitReader(from, maxTakeBytes))
	for {
		text, err := in.ReadBytes('\n')
		got.bytes += int64(len(text))
		if len(text) > 0 && !w.addLine(&got, text, to) {
			return got
		}
		if err != nil {
			got.invalid = got.invalid || !errors.Is(err, io.EOF)
			return got
		}
	}
}

func (w taking) addLine(got *copied, text []byte, to io.Writer) bool {
	var line Line
	if got.end != nil || json.Unmarshal(text, &line) != nil || (line.Call == nil) == (line.End == nil) {
		got.invalid = true
		return false
	}
	if line.End != nil {
		got.end = line.End
		return true
	}
	clean := Clean(w.record, *line.Call)
	got.skeleton = append(got.skeleton, Call{Seq: clean.Seq, Caller: clean.Caller, Target: clean.Target, Started: clean.Started, Replied: clean.Replied, Finished: clean.Finished, Span: clean.Span})
	stored, proven := Prove(w.record, clean)
	if !proven {
		got.withheld++
	}
	encoded, err := json.Marshal(Line{Call: &stored})
	if err == nil {
		_, err = to.Write(append(encoded, '\n'))
	}
	if err != nil {
		got.failed = true
		return false
	}
	got.heads = append(got.heads, headOf(stored, got.offset, int64(len(encoded))))
	got.offset += int64(len(encoded)) + 1
	return true
}

func headOf(c Call, offset, length int64) Head {
	head := Head{
		Seq: c.Seq, Caller: c.Caller, Target: c.Target, Port: c.Port, Method: c.Method, Path: c.Path,
		Status: c.Status, Outcome: c.Outcome, Withheld: c.Withheld, Started: c.Started, Offset: offset, Length: length,
	}
	if !c.Finished.IsZero() {
		head.DurationMs = c.Finished.Sub(c.Started).Milliseconds()
	}
	return head
}

func (w taking) restarted(since time.Time) bool {
	if w.in.Name != SuiteTake || since.IsZero() || w.in.Result == nil {
		return false
	}
	for _, s := range w.in.Result.Steps {
		if started, err := time.Parse(time.RFC3339Nano, s.StartedAt); err == nil && since.After(started) {
			return true
		}
	}
	return false
}

func (w taking) store(got copied) (Status, bool) {
	links := LinkCalls(got.skeleton, w.in.Result)
	for i := range got.heads {
		got.heads[i].Link = links[i]
	}
	status := Status{
		Take: w.in.Name, State: StateTaken, Calls: len(got.heads), WithheldCalls: got.withheld,
		BodiesOmitted: got.end.BodiesOmitted, CallsDropped: got.end.CallsDropped,
	}
	index := Index{Status: status, Steps: stepRefs(w.in.Result), Heads: got.heads, PublicFields: w.in.PublicFields, Routes: w.in.Routes}
	if index.Heads == nil {
		index.Heads = []Head{}
	}
	data, err := json.Marshal(index)
	if err != nil {
		return w.unstored()
	}
	final := filepath.Join(w.root, w.in.Name)
	if os.WriteFile(filepath.Join(w.partial(), IndexFile), data, fileMode) != nil || os.RemoveAll(final) != nil || os.Rename(w.partial(), final) != nil {
		return w.unstored()
	}
	return status, false
}

func stepRefs(result *contract.Result) []StepRef {
	refs := []StepRef{}
	if result == nil {
		return refs
	}
	for _, s := range result.Steps {
		refs = append(refs, StepRef{Flow: s.Flow, Step: s.ID, Status: s.Status})
	}
	return refs
}
