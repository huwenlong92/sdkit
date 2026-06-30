package reporter

import (
	"context"
	"fmt"
	"time"
)

type ErrorHandler func(ctx context.Context, event Event, err SinkError)

type Reporter struct {
	sinks   []sinkRegistration
	onError ErrorHandler
}

type Option func(*Reporter)

func New(sinks ...Sink) *Reporter {
	r := &Reporter{}
	for _, sink := range sinks {
		r.Use(sink)
	}
	return r
}

func NewWithOptions(options ...Option) *Reporter {
	r := &Reporter{}
	for _, option := range options {
		if option != nil {
			option(r)
		}
	}
	return r
}

func WithSink(sink Sink, options ...SinkOption) Option {
	return func(r *Reporter) {
		r.Use(sink, options...)
	}
}

func WithErrorHandler(handler ErrorHandler) Option {
	return func(r *Reporter) {
		r.onError = handler
	}
}

func (r *Reporter) Use(sink Sink, options ...SinkOption) *Reporter {
	if r == nil || sink == nil {
		return r
	}
	r.sinks = append(r.sinks, newSinkRegistration(sink, options...))
	return r
}

func (r *Reporter) Emit(ctx context.Context, event Event) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if event.At.IsZero() {
		event.At = now()
	}
	event = event.Clone()
	allErrors := make(SinkErrors, 0)
	for _, reg := range r.sinks {
		if reg.sink == nil {
			continue
		}
		if reg.filter != nil && !reg.filter(event) {
			continue
		}
		if err := handleSink(ctx, reg.sink, event); err != nil {
			sinkErr := SinkError{Sink: reg.name, Required: reg.required, Err: err}
			allErrors = append(allErrors, sinkErr)
			if r.onError != nil {
				r.onError(ctx, event, sinkErr)
			}
		}
	}
	required := allErrors.RequiredOnly()
	if len(required) > 0 {
		return required
	}
	return nil
}

func (r *Reporter) Log(ctx context.Context, event Event) error {
	return r.Emit(ctx, event.WithKind(KindLog))
}

func (r *Reporter) Progress(ctx context.Context, event Event) error {
	return r.Emit(ctx, event.WithKind(KindProgress))
}

func (r *Reporter) Done(ctx context.Context, event Event) error {
	return r.Emit(ctx, event.WithKind(KindDone))
}

func (r *Reporter) Failed(ctx context.Context, event Event) error {
	return r.Emit(ctx, event.WithKind(KindFailed))
}

func (r *Reporter) Canceled(ctx context.Context, event Event) error {
	return r.Emit(ctx, event.WithKind(KindCanceled))
}

func handleSink(ctx context.Context, sink Sink, event Event) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("reporter sink panic: %v", recovered)
		}
	}()
	return sink.Handle(ctx, event)
}

var now = time.Now
