package tests

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/tracking"
)

func TestLoggerContextFieldsIncludeTrackID(t *testing.T) {
	ctx := tracking.WithTrackID(context.Background(), "track-logger")

	fields := logger.ContextFields(ctx)
	for _, field := range fields {
		if field.Key == logger.TrackIDKey && field.String == "track-logger" {
			return
		}
	}

	t.Fatalf("missing track_id field: %+v", fields)
}
