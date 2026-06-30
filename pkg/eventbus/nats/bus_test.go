//go:build sdkit_eventbus_nats

package nats

import (
	"context"
	"testing"
	"time"
)

func TestPublishFlushContextAddsDeadlineWhenMissing(t *testing.T) {
	ctx, cancel := publishFlushContext(context.Background())
	defer cancel()

	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("publish flush context should add a deadline")
	}
}

func TestPublishFlushContextKeepsExistingDeadline(t *testing.T) {
	wantDeadline := time.Now().Add(time.Minute)
	baseCtx, baseCancel := context.WithDeadline(context.Background(), wantDeadline)
	defer baseCancel()

	ctx, cancel := publishFlushContext(baseCtx)
	defer cancel()

	gotDeadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("publish flush context should keep an existing deadline")
	}
	if !gotDeadline.Equal(wantDeadline) {
		t.Fatalf("deadline: want %s, got %s", wantDeadline, gotDeadline)
	}
}
