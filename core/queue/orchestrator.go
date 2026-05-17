package queue

import (
	"context"
	"errors"
	"time"
)

type RuntimeExecution struct {
	Pattern     string
	Handler     HandlerFunc
	Middlewares []RuntimeMiddleware
	Hooks       []Hook
	Message     *Message
	Metadata    HandlerMetadata
}

type OrchestratorOption func(*Orchestrator)

type Orchestrator struct {
	publishers []EventPublisher
	observers  []Observer
}

func NewOrchestrator(opts ...OrchestratorOption) *Orchestrator {
	o := &Orchestrator{}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	return o
}

func WithEventPublisher(publisher EventPublisher) OrchestratorOption {
	return func(o *Orchestrator) {
		if o != nil && publisher != nil {
			o.publishers = append(o.publishers, publisher)
		}
	}
}

func WithObserver(observer Observer) OrchestratorOption {
	return func(o *Orchestrator) {
		if o != nil && observer != nil {
			o.observers = append(o.observers, observer)
		}
	}
}

func (o *Orchestrator) AddEventPublisher(publisher EventPublisher) {
	if o == nil || publisher == nil {
		return
	}
	o.publishers = append(o.publishers, publisher)
}

func (o *Orchestrator) AddObserver(observer Observer) {
	if o == nil || observer == nil {
		return
	}
	o.observers = append(o.observers, observer)
}

func (o *Orchestrator) Execute(ctx context.Context, exec RuntimeExecution) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if exec.Handler == nil {
		return ErrNotInitialized
	}
	msg := exec.Message
	if msg != nil {
		if msg.State == "" {
			TransitionTaskState(msg, TaskPending)
		}
		TransitionTaskState(msg, TaskRunning)
		ctx = EnsureMessageRuntimeContext(ctx, msg)
		ctx = ContextWithMessage(ctx, msg)
	}

	o.publish(ctx, RuntimeEventTaskStarted, msg, nil)
	o.observeStart(ctx, msg)

	err := runBeforeProcess(ctx, msg, exec.Hooks)
	if err == nil {
		err = ChainRuntime(exec.Handler, normalizeRuntimeMiddlewares(exec.Middlewares)...)(ctx, msg)
	}
	runAfterProcess(ctx, msg, err, exec.Hooks)
	if err != nil {
		o.finishFailure(ctx, msg, err)
		runOnFailure(ctx, msg, err, exec.Hooks)
		return err
	}

	TransitionTaskState(msg, TaskSuccess)
	o.publish(ctx, RuntimeEventTaskSuccess, msg, nil)
	o.observeFinish(ctx, msg, nil)
	runOnSuccess(ctx, msg, exec.Hooks)
	return nil
}

func (o *Orchestrator) finishFailure(ctx context.Context, msg *Message, err error) {
	eventType := RuntimeEventTaskFailed
	if msg != nil && msg.State == TaskDeadLetter {
		eventType = RuntimeEventTaskDeadLetter
	} else if IsDeadLetterError(err) || IsFatalError(err) || shouldMarkDeadLetter(msg, err) {
		TransitionTaskState(msg, TaskFailed)
		TransitionTaskState(msg, TaskDeadLetter)
		eventType = RuntimeEventTaskDeadLetter
	} else if IsTimeoutError(err) || errors.Is(err, context.DeadlineExceeded) {
		TransitionTaskState(msg, TaskFailed)
		eventType = RuntimeEventTaskTimeout
	} else if IsRetryableError(err) || IsRateLimitError(err) {
		TransitionTaskState(msg, TaskRetrying)
		eventType = RuntimeEventTaskRetry
	} else {
		TransitionTaskState(msg, TaskFailed)
	}

	o.publish(ctx, eventType, msg, err)
	if eventType == RuntimeEventTaskRetry {
		o.observeRetry(ctx, msg, err)
	} else {
		o.observeFailure(ctx, msg, err)
	}
	o.observeFinish(ctx, msg, err)
}

func shouldMarkDeadLetter(msg *Message, err error) bool {
	if msg == nil || err == nil || IsRateLimitError(err) || IsRetryableError(err) || IsIgnoredError(err) {
		return false
	}
	return msg.MaxRetry > 0 && msg.RetryCount >= msg.MaxRetry
}

func (o *Orchestrator) publish(ctx context.Context, eventType RuntimeEventType, msg *Message, err error) {
	if o == nil || len(o.publishers) == 0 {
		return
	}
	event := RuntimeEvent{
		Type:      eventType,
		Message:   msg,
		Error:     err,
		Timestamp: time.Now(),
	}
	for _, publisher := range o.publishers {
		if publisher != nil {
			publisher.Publish(ctx, event)
		}
	}
}

func (o *Orchestrator) observeStart(ctx context.Context, msg *Message) {
	if o == nil {
		return
	}
	for _, observer := range o.observers {
		if observer != nil {
			observer.OnTaskStart(ctx, msg)
		}
	}
}

func (o *Orchestrator) observeFinish(ctx context.Context, msg *Message, err error) {
	if o == nil {
		return
	}
	for _, observer := range o.observers {
		if observer != nil {
			observer.OnTaskFinish(ctx, msg, err)
		}
	}
}

func (o *Orchestrator) observeRetry(ctx context.Context, msg *Message, err error) {
	if o == nil {
		return
	}
	for _, observer := range o.observers {
		if observer != nil {
			observer.OnTaskRetry(ctx, msg, err)
		}
	}
}

func (o *Orchestrator) observeFailure(ctx context.Context, msg *Message, err error) {
	if o == nil {
		return
	}
	for _, observer := range o.observers {
		if observer != nil {
			observer.OnTaskFailure(ctx, msg, err)
		}
	}
}
