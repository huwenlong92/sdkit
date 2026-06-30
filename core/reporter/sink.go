package reporter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type Sink interface {
	Handle(ctx context.Context, event Event) error
}

type SinkFunc func(ctx context.Context, event Event) error

func (f SinkFunc) Handle(ctx context.Context, event Event) error {
	if f == nil {
		return nil
	}
	return f(ctx, event)
}

type Filter func(Event) bool

type SinkOption func(*sinkRegistration)

func WithName(name string) SinkOption {
	return func(reg *sinkRegistration) {
		reg.name = strings.TrimSpace(name)
	}
}

func WithRequired() SinkOption {
	return func(reg *sinkRegistration) {
		reg.required = true
	}
}

func WithFilter(filter Filter) SinkOption {
	return func(reg *sinkRegistration) {
		reg.filter = filter
	}
}

func KindFilter(kinds ...Kind) Filter {
	allowed := make(map[Kind]struct{}, len(kinds))
	for _, kind := range kinds {
		allowed[kind] = struct{}{}
	}
	return func(event Event) bool {
		_, ok := allowed[event.Kind]
		return ok
	}
}

func TerminalFilter() Filter {
	return func(event Event) bool {
		return event.IsTerminal()
	}
}

func AndFilter(filters ...Filter) Filter {
	return func(event Event) bool {
		for _, filter := range filters {
			if filter != nil && !filter(event) {
				return false
			}
		}
		return true
	}
}

type SinkError struct {
	Sink     string
	Required bool
	Err      error
}

func (e SinkError) Error() string {
	if e.Err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %v", e.Sink, e.Err)
}

func (e SinkError) Unwrap() error {
	return e.Err
}

type SinkErrors []SinkError

func (e SinkErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e))
	for _, item := range e {
		if item.Err != nil {
			parts = append(parts, item.Error())
		}
	}
	return strings.Join(parts, "; ")
}

func (e SinkErrors) Unwrap() []error {
	errs := make([]error, 0, len(e))
	for _, item := range e {
		if item.Err != nil {
			errs = append(errs, item.Err)
		}
	}
	return errs
}

func (e SinkErrors) RequiredOnly() SinkErrors {
	out := make(SinkErrors, 0, len(e))
	for _, item := range e {
		if item.Required {
			out = append(out, item)
		}
	}
	return out
}

func IsSinkError(err error) bool {
	var sinkErr SinkError
	return errors.As(err, &sinkErr)
}

type sinkRegistration struct {
	name     string
	sink     Sink
	filter   Filter
	required bool
}

func newSinkRegistration(sink Sink, options ...SinkOption) sinkRegistration {
	reg := sinkRegistration{sink: sink, name: sinkName(sink)}
	for _, option := range options {
		if option != nil {
			option(&reg)
		}
	}
	return reg
}

func sinkName(sink Sink) string {
	if sink == nil {
		return "nil"
	}
	value := reflect.TypeOf(sink)
	for value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Name() != "" {
		return value.Name()
	}
	return value.String()
}
