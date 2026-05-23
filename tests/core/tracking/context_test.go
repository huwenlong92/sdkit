package tests

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/tracking"
)

func TestWithTrackIDAndTrackID(t *testing.T) {
	ctx := tracking.WithTrackID(context.Background(), "track-1")
	if got := tracking.TrackID(ctx); got != "track-1" {
		t.Fatalf("track_id: want track-1, got %s", got)
	}
}

func TestMustTrackIDGeneratesWhenMissing(t *testing.T) {
	if got := tracking.MustTrackID(context.Background()); got == "" {
		t.Fatal("track_id should not be empty")
	}
}
