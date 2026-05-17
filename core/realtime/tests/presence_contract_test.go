package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/realtime"
)

var _ realtime.Presence = (*contractPresence)(nil)

func TestPresenceContractRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	presence := &contractPresence{}
	if err := presence.Online(ctx, "c1", realtime.NewUserIdentity("10", 0)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Presence Online canceled error: %v", err)
	}
}

func TestPresenceContractRejectsAfterClose(t *testing.T) {
	ctx := context.Background()

	presence := &contractPresence{}
	if err := presence.Close(); err != nil {
		t.Fatalf("presence close: %v", err)
	}
	if _, err := presence.IsOnline(ctx, "c1"); !errors.Is(err, realtime.ErrClosed) {
		t.Fatalf("IsOnline after close: %v", err)
	}
}

type contractPresence struct {
	closed bool
}

func (p *contractPresence) Close() error {
	p.closed = true
	return nil
}

func (p *contractPresence) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.closed {
		return realtime.ErrClosed
	}
	return nil
}

func (p *contractPresence) Online(ctx context.Context, _ string, _ *realtime.Identity) error {
	return p.check(ctx)
}

func (p *contractPresence) Offline(ctx context.Context, _ string) error {
	return p.check(ctx)
}

func (p *contractPresence) Heartbeat(ctx context.Context, _ string) error {
	return p.check(ctx)
}

func (p *contractPresence) IsOnline(ctx context.Context, _ string) (bool, error) {
	if err := p.check(ctx); err != nil {
		return false, err
	}
	return false, nil
}
