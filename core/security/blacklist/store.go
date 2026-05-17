package blacklist

import "context"

type Store interface {
	Add(ctx context.Context, entry Entry) error
	Contains(ctx context.Context, typ string, value string) (bool, error)
	Remove(ctx context.Context, typ string, value string) error
}
