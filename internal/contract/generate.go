package contract

import "reflect"

//go:generate go run ./gen -root ../..

const (
	NamePattern        = `^[a-z0-9][a-z0-9-]{0,62}$`
	RelPathPattern     = `^(?:[^/.][^/]*|\.[^/.][^/]*|\.\.[^/]+)(?:/(?:[^/.][^/]*|\.[^/.][^/]*|\.\.[^/]+))*$`
	PinnedImagePattern = `^[^@\s]+@sha256:[0-9a-f]{64}$`
	ImageKeyPattern    = `^IMG_[A-Z0-9_]+$`
)

type Root struct {
	Name string
	Type reflect.Type
}

type Enum struct {
	Type   reflect.Type
	Values []any
}

type Pattern struct {
	Type    reflect.Type
	Pattern string
}

func Roots() []Root {
	return []Root{
		root[Manifest]("manifest"),
		root[Result]("result"),
		root[Ledger]("ledger"),
		root[Context]("context"),
		root[ExecResult]("exec"),
		root[Summary]("summary"),
		root[BundleMeta]("meta"),
		root[Declaration]("declaration"),
		root[Signal]("signal"),
	}
}

func Enums() []Enum {
	return []Enum{
		enum(SchemaVersion1, SchemaVersion2),
		enum(StepKindRequest, StepKindWait, StepKindCapture),
		enum(StepStatusNotRun, StepStatusRunning, StepStatusPassed, StepStatusFailed, StepStatusSkipped),
		enum(FlowStatusStarted, FlowStatusPassed, FlowStatusFailed, FlowStatusSkipped),
		enum(SummaryFormat3),
		enum(VerdictPassed, VerdictFailed, VerdictError, VerdictAborted),
		enum(SealPassed, SealFailed, SealError),
		enum(FlowOutcomePassed, FlowOutcomeFailed, FlowOutcomeSkipped, FlowOutcomeNotRun, FlowOutcomeIncomplete),
		enum(CollectStageOK, CollectStageSkipped, CollectStageAbsent, CollectStageError),
		enum(OpenBrowser, OpenLink, OpenCopy),
		enum(DriverMySQL, DriverPostgres),
		enum(SecretFormatDotenv, SecretFormatJSON, SecretFormatPEM),
		enum(SignalKindMetric, SignalKindEvent, SignalKindSpan),
		enum(TimeSourceHost, TimeSourceContainer, TimeSourceRunner),
		enum(ToneOK, ToneWarn, ToneBad, ToneIdle, ToneInfo),
		enum(SpanStatusOK, SpanStatusError, SpanStatusIncomplete),
	}
}

func Patterns() []Pattern {
	return []Pattern{
		pattern[Name](NamePattern),
		pattern[RelPath](RelPathPattern),
		pattern[PinnedImage](PinnedImagePattern),
		pattern[ImageKeyName](ImageKeyPattern),
	}
}

func root[T any](name string) Root {
	return Root{Name: name, Type: reflect.TypeFor[T]()}
}

func pattern[T ~string](p string) Pattern {
	return Pattern{Type: reflect.TypeFor[T](), Pattern: p}
}

func enum[T ~string | ~int](values ...T) Enum {
	plain := make([]any, len(values))
	for i, v := range values {
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.String {
			plain[i] = rv.String()
		} else {
			plain[i] = int(rv.Int())
		}
	}
	return Enum{Type: reflect.TypeFor[T](), Values: plain}
}
