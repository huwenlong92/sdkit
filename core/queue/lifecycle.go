package queue

import "context"

type Hook interface {
	BeforeProcess(ctx context.Context, msg *Message) error
	AfterProcess(ctx context.Context, msg *Message, err error)
	OnSuccess(ctx context.Context, msg *Message)
	OnFailure(ctx context.Context, msg *Message, err error)
}

type HookFunc struct {
	Before  func(context.Context, *Message) error
	After   func(context.Context, *Message, error)
	Success func(context.Context, *Message)
	Failure func(context.Context, *Message, error)
}

func (h HookFunc) BeforeProcess(ctx context.Context, msg *Message) error {
	if h.Before == nil {
		return nil
	}
	return h.Before(ctx, msg)
}

func (h HookFunc) AfterProcess(ctx context.Context, msg *Message, err error) {
	if h.After != nil {
		h.After(ctx, msg, err)
	}
}

func (h HookFunc) OnSuccess(ctx context.Context, msg *Message) {
	if h.Success != nil {
		h.Success(ctx, msg)
	}
}

func (h HookFunc) OnFailure(ctx context.Context, msg *Message, err error) {
	if h.Failure != nil {
		h.Failure(ctx, msg, err)
	}
}
