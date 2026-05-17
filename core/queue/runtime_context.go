package queue

import "context"

// RuntimeContext 只保存 runtime metadata、runtime resources 标识和 runtime state。
//
// 不要把业务 payload、业务对象或外部 SDK 客户端放入 RuntimeContext。
type RuntimeContext struct {
	TraceID   string
	WorkerID  string
	QueueName string
	TaskState TaskState
}

type taskRuntimeContextKey struct{}

func ContextWithRuntimeContext(ctx context.Context, runtime *RuntimeContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if runtime == nil {
		return ctx
	}
	copied := cloneRuntimeContext(runtime)
	return context.WithValue(ctx, taskRuntimeContextKey{}, &copied)
}

func RuntimeContextFromContext(ctx context.Context) (*RuntimeContext, bool) {
	if ctx == nil {
		return nil, false
	}
	runtime, ok := ctx.Value(taskRuntimeContextKey{}).(*RuntimeContext)
	if !ok || runtime == nil {
		return nil, false
	}
	copied := cloneRuntimeContext(runtime)
	return &copied, true
}

func RuntimeContextFromMessage(msg *Message) *RuntimeContext {
	if msg == nil {
		return nil
	}
	runtime := cloneRuntimeContext(msg.Runtime)
	if runtime.TraceID == "" {
		runtime.TraceID = TraceIDFromHeaders(msg.Headers)
	}
	if runtime.QueueName == "" {
		runtime.QueueName = msg.Queue
	}
	if runtime.QueueName == "" {
		if queueName, ok := MessageMetadataString(msg, MessageMetadataQueue); ok {
			runtime.QueueName = queueName
		}
	}
	if runtime.WorkerID == "" {
		if worker, ok := MessageMetadataString(msg, MessageMetadataWorker); ok {
			runtime.WorkerID = worker
		}
	}
	if runtime.TaskState == "" {
		runtime.TaskState = msg.State
	}
	return &runtime
}

func EnsureMessageRuntimeContext(ctx context.Context, msg *Message) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if msg == nil {
		return ctx
	}
	if msg.Runtime == nil {
		msg.Runtime = RuntimeContextFromMessage(msg)
	} else {
		runtime := RuntimeContextFromMessage(msg)
		msg.Runtime = runtime
	}
	return ContextWithRuntimeContext(ctx, msg.Runtime)
}

func cloneRuntimeContext(runtime *RuntimeContext) RuntimeContext {
	if runtime == nil {
		return RuntimeContext{}
	}
	return RuntimeContext{
		TraceID:   runtime.TraceID,
		WorkerID:  runtime.WorkerID,
		QueueName: runtime.QueueName,
		TaskState: runtime.TaskState,
	}
}
