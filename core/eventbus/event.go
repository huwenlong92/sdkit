package eventbus

import (
	"context"
	"time"

	"github.com/huwenlong92/sdkit/core/tracing"

	"github.com/google/uuid"
)

type Event struct {
	ID        string            `json:"id"`
	Topic     string            `json:"topic"`
	Headers   map[string]string `json:"headers,omitempty"`
	Payload   []byte            `json:"payload,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

func NewEvent(ctx context.Context, topic string, payload []byte, headers map[string]string) (*Event, error) {
	if topic == "" {
		return nil, ErrEmptyTopic
	}
	return &Event{
		ID:        uuid.NewString(),
		Topic:     topic,
		Headers:   eventHeadersFromContext(ctx, headers),
		Payload:   append([]byte(nil), payload...),
		Timestamp: time.Now(),
	}, nil
}

func NewJSONEvent(ctx context.Context, topic string, payload any, headers map[string]string) (*Event, error) {
	data, err := (JSONCodec{}).Marshal(payload)
	if err != nil {
		return nil, err
	}
	return NewEvent(ctx, topic, data, headers)
}

func ContextWithEvent(ctx context.Context, event *Event) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if event == nil {
		return ctx
	}
	return tracing.ContextFromHeaders(ctx, event.Headers)
}

func eventHeadersFromContext(ctx context.Context, headers map[string]string) map[string]string {
	out := tracing.HeadersFromContext(ctx)
	for key, value := range headers {
		if out == nil {
			out = map[string]string{}
		}
		out[key] = value
	}
	return out
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}
