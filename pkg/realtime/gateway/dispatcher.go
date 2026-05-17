package gateway

import (
	"context"

	"github.com/huwenlong92/sdkit/core/realtime"
)

type Dispatcher struct {
	registry realtime.Registry
}

func NewDispatcher(registry realtime.Registry) *Dispatcher {
	return &Dispatcher{registry: registry}
}

func (d *Dispatcher) DispatchEvent(ctx context.Context, evt *realtime.Event) error {
	return d.DispatchLocal(ctx, evt)
}

func (d *Dispatcher) DispatchLocal(ctx context.Context, evt *realtime.Event) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if evt == nil {
		return realtime.ErrEmptyEvent
	}
	evt.Normalize()
	target := evt.Target
	if target == nil {
		return d.Broadcast(ctx, evt)
	}
	switch target.Type {
	case realtime.TargetUser:
		userID := target.UserID
		if userID == "" {
			userID = target.ID
		}
		return d.PushUser(ctx, userID, evt)
	case realtime.TargetRoom:
		roomID := target.RoomID
		if roomID == "" {
			roomID = target.ID
		}
		return d.PushRoom(ctx, roomID, evt)
	case realtime.TargetBroadcast:
		return d.Broadcast(ctx, evt)
	case "client":
		clientID := target.ClientID
		if clientID == "" {
			clientID = target.ID
		}
		return d.DispatchClient(ctx, clientID, evt)
	default:
		return realtime.ErrUnsupportedTopic
	}
}

func (d *Dispatcher) DispatchClient(ctx context.Context, clientID string, evt *realtime.Event) error {
	return d.PushClient(ctx, clientID, evt)
}

func (d *Dispatcher) PushUser(ctx context.Context, userID string, evt *realtime.Event) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if d == nil || d.registry == nil {
		return realtime.ErrNilClient
	}
	for _, client := range d.registry.GetUserClients(userID) {
		_ = pushClient(ctx, client, evt)
	}
	return nil
}

func (d *Dispatcher) PushClient(ctx context.Context, clientID string, evt *realtime.Event) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if d == nil || d.registry == nil {
		return realtime.ErrNilClient
	}
	client, ok := d.registry.Get(clientID)
	if !ok {
		return realtime.ErrClientNotFound
	}
	return pushClient(ctx, client, evt)
}

func (d *Dispatcher) PushRoom(ctx context.Context, roomID string, evt *realtime.Event) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if d == nil || d.registry == nil {
		return realtime.ErrNilClient
	}
	for _, client := range d.registry.GetRoomClients(roomID) {
		_ = pushClient(ctx, client, evt)
	}
	return nil
}

func (d *Dispatcher) Broadcast(ctx context.Context, evt *realtime.Event) error {
	return d.PushRoom(ctx, "", evt)
}

func pushClient(ctx context.Context, client *realtime.Client, evt *realtime.Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return realtime.ErrNilClient
	}
	if client.Ch == nil {
		return realtime.ErrClientChannelUnavailable
	}
	event := realtime.Event{}
	if evt != nil {
		event = *evt.Normalize()
	}
	select {
	case client.Ch <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return realtime.ErrClientBufferFull
	}
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

var _ realtime.Dispatcher = (*Dispatcher)(nil)
