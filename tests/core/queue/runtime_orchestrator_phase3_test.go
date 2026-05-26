package queue_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"
)

func TestDispatcherStageSystemOrdersGlobalAndLocalMiddleware(t *testing.T) {
	dispatcher := queue.NewDispatcher()
	order := make([]string, 0, 5)
	middleware := func(name string) queue.Middleware {
		return func(next queue.HandlerFunc) queue.HandlerFunc {
			return func(ctx context.Context, msg *queue.Message) error {
				order = append(order, name)
				return next(ctx, msg)
			}
		}
	}

	dispatcher.UseRuntime(
		queue.StageMiddleware(queue.DeadLetterStage, middleware("deadletter")),
		queue.StageMiddleware(queue.RecoverStage, middleware("recover")),
	)
	if err := dispatcher.Register(
		"task.stage",
		queue.StageMiddleware(queue.RateLimitStage, middleware("rate")),
		queue.StageMiddleware(queue.TraceStage, middleware("trace")),
		func(context.Context, *queue.Message) error {
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
	dispatcher := queue.NewDispatcher()
	publisher := &recordingPublisher{}
	observer := &recordingObserver{}
	dispatcher.SetOrchestrator(queue.NewOrchestrator(
		queue.WithEventPublisher(publisher),
		queue.WithObserver(observer),
	))

	if err := dispatcher.Register("task.success", func(ctx context.Context, msg *queue.Message) error {
		if msg.State != queue.TaskRunning {
			t.Fatalf("handler state = %s, want %s", msg.State, queue.TaskRunning)
		}
		runtimeCtx, ok := queue.RuntimeContextFromContext(ctx)
		if !ok || runtimeCtx.TaskState != queue.TaskRunning || runtimeCtx.QueueName != "critical" {
			t.Fatalf("runtime context = %#v, ok=%v", runtimeCtx, ok)
		}
		return nil
	}, queue.WithQueue("critical")); err != nil {
		t.Fatalf("register: %v", err)
	}

	msg := &queue.Message{ID: "task-1", Type: "task.success"}
	if err := dispatcher.Dispatch(context.Background(), "task.success", msg); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if msg.State != queue.TaskSuccess || msg.Runtime == nil || msg.Runtime.TaskState != queue.TaskSuccess {
		t.Fatalf("message runtime state = %s %#v", msg.State, msg.Runtime)
	}
	if got, want := publisher.types(), []queue.RuntimeEventType{queue.RuntimeEventTaskStarted, queue.RuntimeEventTaskSuccess}; !reflect.DeepEqual(got, want) {
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
	dispatcher := queue.NewDispatcher()
	publisher := &recordingPublisher{}
	observer := &recordingObserver{}
	dispatcher.SetOrchestrator(queue.NewOrchestrator(
		queue.WithEventPublisher(publisher),
		queue.WithObserver(observer),
	))
	wantErr := errors.New("retry later")
	if err := dispatcher.Register("task.retry", func(context.Context, *queue.Message) error {
		return queue.RetryableAfter(3*time.Second, wantErr)
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	msg := &queue.Message{ID: "task-1", Type: "task.retry"}
	err := dispatcher.Dispatch(context.Background(), "task.retry", msg)
	if !errors.Is(err, wantErr) {
		t.Fatalf("dispatch error = %v, want %v", err, wantErr)
	}
	if msg.State != queue.TaskRetrying {
		t.Fatalf("message state = %s, want %s", msg.State, queue.TaskRetrying)
	}
	if got, want := publisher.types(), []queue.RuntimeEventType{queue.RuntimeEventTaskStarted, queue.RuntimeEventTaskRetry}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if observer.retried != 1 || observer.failed != 0 || observer.finished != 1 {
		t.Fatalf("observer = %+v", observer)
	}
}

func TestRuntimeErrorKinds(t *testing.T) {
	err := queue.NewFatalError(errors.New("fatal"))
	runtimeErr, ok := queue.RuntimeErrorFrom(err)
	if !ok || runtimeErr.Kind != queue.ErrorFatal || !queue.IsFatalError(err) {
		t.Fatalf("fatal runtime error = %#v, ok=%v", runtimeErr, ok)
	}
	err = queue.NewIgnoredError(errors.New("ignored"))
	runtimeErr, ok = queue.RuntimeErrorFrom(err)
	if !ok || runtimeErr.Kind != queue.ErrorIgnored || !queue.IsIgnoredError(err) {
		t.Fatalf("ignored runtime error = %#v, ok=%v", runtimeErr, ok)
	}
}

func TestTaskStateTransitionsRejectInvalidFlow(t *testing.T) {
	msg := &queue.Message{Type: "task.state"}

	if queue.TransitionTaskState(msg, queue.TaskSuccess) {
		t.Fatal("empty state should not transition directly to success")
	}
	if !queue.TransitionTaskState(msg, queue.TaskPending) ||
		!queue.TransitionTaskState(msg, queue.TaskRunning) ||
		!queue.TransitionTaskState(msg, queue.TaskFailed) ||
		!queue.TransitionTaskState(msg, queue.TaskDeadLetter) {
		t.Fatalf("valid state transition failed, msg=%+v", msg)
	}
	if queue.TransitionTaskState(msg, queue.TaskRunning) {
		t.Fatal("deadletter state should not transition back to running")
	}
	if msg.Runtime == nil || msg.Runtime.TaskState != queue.TaskDeadLetter {
		t.Fatalf("runtime task state = %#v", msg.Runtime)
	}
}

type recordingPublisher struct {
	events []queue.RuntimeEvent
}

func (p *recordingPublisher) Publish(_ context.Context, event queue.RuntimeEvent) {
	p.events = append(p.events, event)
}

func (p *recordingPublisher) types() []queue.RuntimeEventType {
	out := make([]queue.RuntimeEventType, 0, len(p.events))
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

func (o *recordingObserver) OnTaskStart(context.Context, *queue.Message) {
	o.started++
}

func (o *recordingObserver) OnTaskFinish(context.Context, *queue.Message, error) {
	o.finished++
}

func (o *recordingObserver) OnTaskRetry(context.Context, *queue.Message, error) {
	o.retried++
}

func (o *recordingObserver) OnTaskFailure(context.Context, *queue.Message, error) {
	o.failed++
}
