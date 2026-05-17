package audit

import "context"

type Writer interface {
	Write(ctx context.Context, event *Event) error
	WriteBatch(ctx context.Context, events []*Event) error
}

type NopWriter struct{}

func (NopWriter) Write(ctx context.Context, event *Event) error {
	return ctx.Err()
}

func (NopWriter) WriteBatch(ctx context.Context, events []*Event) error {
	return ctx.Err()
}
