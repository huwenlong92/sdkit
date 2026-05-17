package eventbus

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	coreeventbus "github.com/huwenlong92/sdkit/core/eventbus"
	"github.com/huwenlong92/sdkit/core/realtime"
	"github.com/huwenlong92/sdkit/core/tracing"
)

type Publisher struct {
	bus   coreeventbus.Bus
	topic string
}

func New(bus coreeventbus.Bus, topic string) *Publisher {
	if topic == "" {
		topic = realtime.DefaultTopic
	}
	return &Publisher{bus: bus, topic: topic}
}

func (p *Publisher) PushUser(ctx context.Context, userID string, evt *realtime.Event) error {
	evt = cloneEvent(evt).Normalize()
	evt.Target = &realtime.Target{
		Type:   realtime.TargetUser,
		ID:     strings.TrimSpace(userID),
		UserID: strings.TrimSpace(userID),
	}
	return p.publish(ctx, evt)
}

func (p *Publisher) PushRoom(ctx context.Context, roomID string, evt *realtime.Event) error {
	evt = cloneEvent(evt).Normalize()
	evt.RoomID = strings.TrimSpace(roomID)
	evt.Target = &realtime.Target{
		Type:   realtime.TargetRoom,
		ID:     evt.RoomID,
		RoomID: evt.RoomID,
	}
	return p.publish(ctx, evt)
}

func (p *Publisher) Broadcast(ctx context.Context, evt *realtime.Event) error {
	evt = cloneEvent(evt).Normalize()
	evt.Target = &realtime.Target{Type: realtime.TargetBroadcast}
	return p.publish(ctx, evt)
}

func (p *Publisher) publish(ctx context.Context, evt *realtime.Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if p == nil || p.bus == nil {
		return realtime.ErrNilPublisher
	}
	if evt == nil {
		return realtime.ErrEmptyEvent
	}
	evt.Normalize()
	if evt.Action == "" {
		return realtime.ErrEmptyEvent
	}
	if evt.Timestamp == 0 {
		evt.Timestamp = time.Now().Unix()
	}
	if evt.Time == 0 {
		evt.Time = evt.Timestamp
	}
	evt.Headers = mergeHeaders(tracing.HeadersFromContext(ctx), evt.Headers)
	if evt.TraceID == "" {
		evt.TraceID = tracing.TraceID(ctx)
	}
	if len(evt.Payload) == 0 && evt.Data != nil {
		payload, err := json.Marshal(evt.Data)
		if err != nil {
			return err
		}
		evt.Payload = payload
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	event, err := coreeventbus.NewEvent(ctx, p.topic, payload, evt.Headers)
	if err != nil {
		return err
	}
	return p.bus.Publish(ctx, event)
}

func cloneEvent(evt *realtime.Event) *realtime.Event {
	if evt == nil {
		return &realtime.Event{}
	}
	cloned := *evt
	if evt.Headers != nil {
		cloned.Headers = make(map[string]string, len(evt.Headers))
		for key, value := range evt.Headers {
			cloned.Headers[key] = value
		}
	}
	if evt.Payload != nil {
		cloned.Payload = append([]byte(nil), evt.Payload...)
	}
	return &cloned
}

func mergeHeaders(base map[string]string, overrides map[string]string) map[string]string {
	if len(base) == 0 && len(overrides) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(overrides))
	for key, value := range base {
		if key != "" && value != "" {
			merged[key] = value
		}
	}
	for key, value := range overrides {
		if key != "" && value != "" {
			merged[key] = value
		}
	}
	return merged
}

var _ realtime.Publisher = (*Publisher)(nil)
