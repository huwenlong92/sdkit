package tests

import (
	"testing"

	"github.com/huwenlong92/sdkit/core/tracking"
)

func TestNewTrackID(t *testing.T) {
	if got := tracking.NewTrackID(); got == "" {
		t.Fatal("track_id should not be empty")
	}
}
