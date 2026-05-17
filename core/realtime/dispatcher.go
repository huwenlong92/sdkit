package realtime

import "context"

type Dispatcher interface {
	DispatchEvent(ctx context.Context, evt *Event) error
	DispatchLocal(ctx context.Context, evt *Event) error
	DispatchClient(ctx context.Context, clientID string, evt *Event) error
	PushUser(ctx context.Context, userID string, evt *Event) error
	PushClient(ctx context.Context, clientID string, evt *Event) error
	PushRoom(ctx context.Context, roomID string, evt *Event) error
	Broadcast(ctx context.Context, evt *Event) error
}
