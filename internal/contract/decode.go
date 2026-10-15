package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var (
	ErrTrailingData  = errors.New("contract: trailing data after the JSON value")
	ErrSchemaVersion = errors.New("contract: unsupported schemaVersion")
)

func Decode(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return ErrTrailingData
	}
	return nil
}

type runnerFile interface {
	schemaVersion() SchemaVersion
}

func ReadRunnerFile[T any, P interface {
	*T
	runnerFile
}](r io.Reader) (*T, error) {
	v := P(new(T))
	if err := Decode(r, v); err != nil {
		return nil, err
	}
	if got := v.schemaVersion(); got != RunnerSchemaVersion {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrSchemaVersion, got, RunnerSchemaVersion)
	}
	return (*T)(v), nil
}
