package queue

import "context"

type messageContextKey struct{}
type contextMetadataKey struct{}

type ContextMetadata struct {
	TaskID     string
	TaskType   string
	Queue      string
	TaskState  TaskState
	RetryCount int
	MaxRetry   int
	WorkerID   string
	TrackID    string
	RequestID  string
	TraceID    string
	SpanID     string
	TenantID   string
	UserID     string
	Event      map[string]string
}

func ContextWithMessage(ctx context.Context, msg *Message) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if msg == nil {
		return ctx
	}
	ctx = EnsureMessageRuntimeContext(ctx, msg)
	ctx = context.WithValue(ctx, messageContextKey{}, msg)
	return ContextWithMetadata(ctx, ContextMetadataFromMessage(msg))
}

func MessageFromContext(ctx context.Context) (*Message, bool) {
	if ctx == nil {
		return nil, false
	}
	msg, ok := ctx.Value(messageContextKey{}).(*Message)
	return msg, ok && msg != nil
}

func ContextWithMetadata(ctx context.Context, metadata ContextMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextMetadataKey{}, cloneContextMetadata(metadata))
}

func MetadataFromContext(ctx context.Context) (ContextMetadata, bool) {
	if ctx == nil {
		return ContextMetadata{}, false
	}
	metadata, ok := ctx.Value(contextMetadataKey{}).(ContextMetadata)
	if !ok {
		return ContextMetadata{}, false
	}
	return cloneContextMetadata(metadata), true
}

func ContextMetadataFromMessage(msg *Message) ContextMetadata {
	if msg == nil {
		return ContextMetadata{}
	}
	return ContextMetadata{
		TaskID:     msg.ID,
		TaskType:   msg.Type,
		Queue:      msg.Queue,
		TaskState:  msg.State,
		RetryCount: msg.RetryCount,
		MaxRetry:   msg.MaxRetry,
		WorkerID:   runtimeWorkerID(msg),
		TrackID:    TrackIDFromHeaders(msg.Headers),
		RequestID:  RequestIDFromHeaders(msg.Headers),
		TraceID:    TraceIDFromHeaders(msg.Headers),
		SpanID:     SpanIDFromHeaders(msg.Headers),
		TenantID:   CorrelationHeaderValue(msg.Headers, "tenant_id"),
		UserID:     CorrelationHeaderValue(msg.Headers, "user_id"),
		Event:      cloneStringMap(msg.Headers),
	}
}

func runtimeWorkerID(msg *Message) string {
	if msg == nil {
		return ""
	}
	if msg.Runtime != nil && msg.Runtime.WorkerID != "" {
		return msg.Runtime.WorkerID
	}
	worker, _ := MessageMetadataString(msg, MessageMetadataWorker)
	return worker
}

func cloneContextMetadata(metadata ContextMetadata) ContextMetadata {
	metadata.Event = cloneStringMap(metadata.Event)
	return metadata
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
