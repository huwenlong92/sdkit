package realtime

import "context"

type Presence interface {
	Online(ctx context.Context, clientID string, identity *Identity) error
	Offline(ctx context.Context, clientID string) error
	Heartbeat(ctx context.Context, clientID string) error
	IsOnline(ctx context.Context, clientID string) (bool, error)
}
