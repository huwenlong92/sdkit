package queue

import "context"

type Observer interface {
	OnTaskStart(ctx context.Context, msg *Message)
	OnTaskFinish(ctx context.Context, msg *Message, err error)
	OnTaskRetry(ctx context.Context, msg *Message, err error)
	OnTaskFailure(ctx context.Context, msg *Message, err error)
}

type ObserverFunc struct {
	Start   func(context.Context, *Message)
	Finish  func(context.Context, *Message, error)
	Retry   func(context.Context, *Message, error)
	Failure func(context.Context, *Message, error)
}

func (o ObserverFunc) OnTaskStart(ctx context.Context, msg *Message) {
	if o.Start != nil {
		o.Start(ctx, msg)
	}
}

func (o ObserverFunc) OnTaskFinish(ctx context.Context, msg *Message, err error) {
	if o.Finish != nil {
		o.Finish(ctx, msg, err)
	}
}

func (o ObserverFunc) OnTaskRetry(ctx context.Context, msg *Message, err error) {
	if o.Retry != nil {
		o.Retry(ctx, msg, err)
	}
}

func (o ObserverFunc) OnTaskFailure(ctx context.Context, msg *Message, err error) {
	if o.Failure != nil {
		o.Failure(ctx, msg, err)
	}
}
