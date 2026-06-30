package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/reporter"
)

func TestReporterFanOutFiltersAndRequiredErrors(t *testing.T) {
	ctx := context.Background()
	calls := make([]string, 0)
	optionalErr := errors.New("optional failed")
	requiredErr := errors.New("required failed")
	observedErrors := make([]reporter.SinkError, 0)
	r := reporter.NewWithOptions(
		reporter.WithErrorHandler(func(ctx context.Context, event reporter.Event, err reporter.SinkError) {
			observedErrors = append(observedErrors, err)
		}),
		reporter.WithSink(reporter.SinkFunc(func(ctx context.Context, event reporter.Event) error {
			calls = append(calls, "log-only")
			return nil
		}), reporter.WithFilter(reporter.KindFilter(reporter.KindLog))),
		reporter.WithSink(reporter.SinkFunc(func(ctx context.Context, event reporter.Event) error {
			calls = append(calls, "optional")
			return optionalErr
		}), reporter.WithName("optional")),
		reporter.WithSink(reporter.SinkFunc(func(ctx context.Context, event reporter.Event) error {
			calls = append(calls, "required")
			return requiredErr
		}), reporter.WithName("required"), reporter.WithRequired()),
	)

	err := r.Progress(ctx, reporter.Event{OperationID: "op-1"})
	if err == nil {
		t.Fatalf("expected required sink error")
	}
	if !errors.Is(err, requiredErr) {
		t.Fatalf("error = %v, want wrapped required error", err)
	}
	if errors.Is(err, optionalErr) {
		t.Fatalf("optional sink error should not be returned: %v", err)
	}
	if len(calls) != 2 || calls[0] != "optional" || calls[1] != "required" {
		t.Fatalf("calls = %#v, want optional and required only", calls)
	}
	if len(observedErrors) != 2 {
		t.Fatalf("observed errors = %d, want 2", len(observedErrors))
	}
}

func TestReporterRecoversSinkPanic(t *testing.T) {
	r := reporter.NewWithOptions(
		reporter.WithSink(reporter.SinkFunc(func(ctx context.Context, event reporter.Event) error {
			panic("sink exploded")
		}), reporter.WithRequired()),
	)

	err := r.Log(context.Background(), reporter.Event{OperationID: "op-1"})
	if err == nil {
		t.Fatalf("expected panic to be converted to error")
	}
	if !errors.As(err, new(reporter.SinkErrors)) {
		t.Fatalf("error = %T, want reporter.SinkErrors", err)
	}
}
