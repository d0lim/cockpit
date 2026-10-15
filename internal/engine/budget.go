package engine

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/dop251/goja"
)

const (
	DefaultMaxCallStack = 10_000
	overflowFrames      = 10
)

type Budget struct {
	Deadline     time.Duration
	MaxCallStack int
}

type FailureKind string

const (
	FailThrown    FailureKind = "thrown"
	FailDeadline  FailureKind = "deadline"
	FailCanceled  FailureKind = "canceled"
	FailStack     FailureKind = "stack_overflow"
	FailUnsettled FailureKind = "unsettled"
	FailUnhandled FailureKind = "unhandled_rejection"
	FailLoad      FailureKind = "load"
	failInterrupt FailureKind = "interrupted"
)

type Failure struct {
	Kind    FailureKind
	Message string
	Stack   string
}

func (f *Failure) Error() string {
	return string(f.Kind) + ": " + f.Message
}

var errDeadline = errors.New("deadline exceeded")

func (b Budget) normalize() (Budget, error) {
	if b.Deadline <= 0 {
		return b, errors.New("budget deadline must be positive")
	}
	if b.MaxCallStack < 0 {
		return b, errors.New("budget max call stack must not be negative")
	}
	if b.MaxCallStack == 0 {
		b.MaxCallStack = DefaultMaxCallStack
	}
	return b, nil
}

func (h *Host) watch(ctx context.Context, cancel context.CancelCauseFunc, deadline time.Duration) func() {
	timer := time.AfterFunc(deadline, func() { cancel(errDeadline) })
	unwatch := context.AfterFunc(ctx, func() {
		h.rt.Interrupt(errDeadline)
		h.loop.StopNoWait()
	})
	return func() {
		timer.Stop()
		unwatch()
	}
}

func interruption(cause error) *Failure {
	if errors.Is(cause, errDeadline) {
		return &Failure{Kind: FailDeadline, Message: "the run exceeded its deadline"}
	}
	return &Failure{Kind: FailCanceled, Message: "the run was canceled"}
}

func (h *Host) classify(err error) *Failure {
	var failure *Failure
	var interrupted *goja.InterruptedError
	var overflow *goja.StackOverflowError
	var exception *goja.Exception
	switch {
	case errors.As(err, &failure):
		return failure
	case errors.As(err, &interrupted):
		return &Failure{Kind: failInterrupt}
	case errors.As(err, &overflow):
		return &Failure{Kind: FailStack, Message: "maximum call stack size exceeded", Stack: topFrames(overflow.Stack())}
	case errors.As(err, &exception):
		return h.describe(FailThrown, exception.Value())
	}
	return &Failure{Kind: FailLoad, Message: err.Error()}
}

func topFrames(frames []goja.StackFrame) string {
	var b bytes.Buffer
	for i, frame := range frames[:min(overflowFrames, len(frames))] {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("\tat ")
		frame.Write(&b)
	}
	return b.String()
}

func (h *Host) describe(kind FailureKind, reason goja.Value) *Failure {
	f := &Failure{Kind: kind, Message: "a non-error value was thrown"}
	if reason == nil {
		return f
	}
	h.rt.Try(func() {
		f.Message = reason.String()
		if obj, ok := reason.(*goja.Object); ok {
			if message := obj.Get("message"); message != nil && !goja.IsUndefined(message) {
				f.Message = message.String()
			}
			if stack := obj.Get("stack"); stack != nil && !goja.IsUndefined(stack) {
				f.Stack = stack.String()
			}
		}
	})
	return f
}
