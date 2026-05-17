package realtime

import "context"

type Publisher interface {
	PushUser(ctx context.Context, userID string, evt *Event) error
	PushRoom(ctx context.Context, roomID string, evt *Event) error
	Broadcast(ctx context.Context, evt *Event) error
}

type Lifecycle interface {
	Close() error
}

type Runner interface {
	Run(ctx context.Context) error
}

type Service interface {
	Publisher
}
