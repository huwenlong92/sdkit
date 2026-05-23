package queue_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	corequeue "github.com/huwenlong92/sdkit/core/queue"
)

func TestDispatcherStageSystemOrdersGlobalAndLocalMiddleware(t *testing.T) {
	dispatcher := corequeue.NewDispatcher()
	order := make([]string, 0, 5)
	middleware := func(name string) corequeue.Middleware {
		return func(next corequeue.HandlerFunc) corequeue.HandlerFunc {
			return func(ctx context.Context, msg *corequeue.Message) error {
				order = append(order, name)
				return next(ctx, msg)
			}
		}
	}

	dispatcher.UseRuntime(
		corequeue.StageMiddleware(corequeue.DeadLetterStage, middleware("deadletter")),
		corequeue.StageMiddleware(corequeue.RecoverStage, middleware("recover")),
	)
	if err := dispatcher.Register(
		"task.stage",
		corequeue.StageMiddleware(corequeue.RateLimitStage, middleware("rate")),
		corequeue.StageMiddleware(corequeue.TraceStage, middleware("trace")),
		func(context.Context, *corequeue.Message) error {
			order = append(order, "handler")
			return nil
		},
	); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := dispatcher.Dispatch(context.Background(), "task.stage", nil); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	want := []string{"recover", "trace", "rate", "deadletter", "handler"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestOrchestratorPublishesEventsAndRuntimeState(t *testing.T) {
	dispatcher := corequeue.NewDispatcher()
	publisher := &recordingPublisher{}
	observer := &recordingObserver{}
	dispatcher.SetOrchestrator(corequeue.NewOrchestrator(
		corequeue.WithEventPublisher(publisher),
		corequeue.WithObserver(observer),
	))

	if err := dispatcher.Register("task.success", func(ctx context.Context, msg *corequeue.Message) error {
		if msg.State != corequeue.TaskRunning {
			t.Fatalf("handler state = %s, want %s", msg.State, corequeue.TaskRunning)
		}
		runtimeCtx, ok := corequeue.RuntimeContextFromContext(ctx)
		if !ok || runtimeCtx.TaskState != corequeue.TaskRunning || runtimeCtx.QueueName != "critical" {
			t.Fatalf("runtime context = %#v, ok=%v", runtimeCtx, ok)
		}
		return nil
	}, corequeue.WithQueue("critical")); err != nil {
		t.Fatalf("register: %v", err)
	}

	msg := &corequeue.Message{ID: "task-1", Type: "task.success"}
	if err := dispatcher.Dispatch(context.Background(), "task.success", msg); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if msg.State != corequeue.TaskSuccess || msg.Runtime == nil || msg.Runtime.TaskState != corequeue.TaskSuccess {
		t.Fatalf("message runtime state = %s %#v", msg.State, msg.Runtime)
	}
	if got, want := publisher.types(), []corequeue.RuntimeEventType{corequeue.RuntimeEventTaskStarted, corequeue.RuntimeEventTaskSuccess}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if observer.started != 1 || observer.finished != 1 || observer.failed != 0 || observer.retried != 0 {
		t.Fatalf("observer = %+v", observer)
	}
	if publisher.events[0].Timestamp.IsZero() {
		t.Fatal("runtime event timestamp was not set")
	}
}

func TestOrchestratorMarksRetryState(t *testing.T) {
	dispatcher := corequeue.NewDispatcher()
	publisher := &recordingPublisher{}
	observer := &recordingObserver{}
	dispatcher.SetOrchestrator(corequeue.NewOrchestrator(
		corequeue.WithEventPublisher(publisher),
		corequeue.WithObserver(observer),
	))
	wantErr := errors.New("retry later")
	if err := dispatcher.Register("task.retry", func(context.Context, *corequeue.Message) error {
		return corequeue.RetryableAfter(3*time.Second, wantErr)
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	msg := &corequeue.Message{ID: "task-1", Type: "task.retry"}
	err := dispatcher.Dispatch(context.Background(), "task.retry", msg)
	if !errors.Is(err, wantErr) {
		t.Fatalf("dispatch error = %v, want %v", err, wantErr)
	}
	if msg.State != corequeue.TaskRetrying {
		t.Fatalf("message state = %s, want %s", msg.State, corequeue.TaskRetrying)
	}
	if got, want := publisher.types(), []corequeue.RuntimeEventType{corequeue.RuntimeEventTaskStarted, corequeue.RuntimeEventTaskRetry}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if observer.retried != 1 || observer.failed != 0 || observer.finished != 1 {
		t.Fatalf("observer = %+v", observer)
	}
}

func TestRuntimeErrorKinds(t *testing.T) {
	err := corequeue.NewFatalError(errors.New("fatal"))
	runtimeErr, ok := corequeue.RuntimeErrorFrom(err)
	if !ok || runtimeErr.Kind != corequeue.ErrorFatal || !corequeue.IsFatalError(err) {
		t.Fatalf("fatal runtime error = %#v, ok=%v", runtimeErr, ok)
	}
	err = corequeue.NewIgnoredError(errors.New("ignored"))
	runtimeErr, ok = corequeue.RuntimeErrorFrom(err)
	if !ok || runtimeErr.Kind != corequeue.ErrorIgnored || !corequeue.IsIgnoredError(err) {
		t.Fatalf("ignored runtime error = %#v, ok=%v", runtimeErr, ok)
	}
}

func TestTaskStateTransitionsRejectInvalidFlow(t *testing.T) {
	msg := &corequeue.Message{Type: "task.state"}

	if corequeue.TransitionTaskState(msg, corequeue.TaskSuccess) {
		t.Fatal("empty state should not transition directly to success")
	}
	if !corequeue.TransitionTaskState(msg, corequeue.TaskPending) ||
		!corequeue.TransitionTaskState(msg, corequeue.TaskRunning) ||
		!corequeue.TransitionTaskState(msg, corequeue.TaskFailed) ||
		!corequeue.TransitionTaskState(msg, corequeue.TaskDeadLetter) {
		t.Fatalf("valid state transition failed, msg=%+v", msg)
	}
	if corequeue.TransitionTaskState(msg, corequeue.TaskRunning) {
		t.Fatal("deadletter state should not transition back to running")
	}
	if msg.Runtime == nil || msg.Runtime.TaskState != corequeue.TaskDeadLetter {
		t.Fatalf("runtime task state = %#v", msg.Runtime)
	}
}

type recordingPublisher struct {
	events []corequeue.RuntimeEvent
}

func (p *recordingPublisher) Publish(_ context.Context, event corequeue.RuntimeEvent) {
	p.events = append(p.events, event)
}

func (p *recordingPublisher) types() []corequeue.RuntimeEventType {
	out := make([]corequeue.RuntimeEventType, 0, len(p.events))
	for _, event := range p.events {
		out = append(out, event.Type)
	}
	return out
}

type recordingObserver struct {
	started  int
	finished int
	retried  int
	failed   int
}

func (o *recordingObserver) OnTaskStart(context.Context, *corequeue.Message) {
	o.started++
}

func (o *recordingObserver) OnTaskFinish(context.Context, *corequeue.Message, error) {
	o.finished++
}

func (o *recordingObserver) OnTaskRetry(context.Context, *corequeue.Message, error) {
	o.retried++
}

func (o *recordingObserver) OnTaskFailure(context.Context, *corequeue.Message, error) {
	o.failed++
}
