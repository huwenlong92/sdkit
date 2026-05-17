package realtime

import "context"

type Room interface {
	Join(ctx context.Context, roomID string, clientID string) error
	Leave(ctx context.Context, roomID string, clientID string) error
	Clients(ctx context.Context, roomID string) ([]string, error)
	Rooms(ctx context.Context, clientID string) ([]string, error)
}
