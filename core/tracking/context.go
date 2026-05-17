package tracking

import "context"

type contextKey struct{}

func WithTrackID(ctx context.Context, trackID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if trackID == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, trackID)
}

func TrackID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if trackID, ok := ctx.Value(contextKey{}).(string); ok && trackID != "" {
		return trackID
	}
	return ""
}

func MustTrackID(ctx context.Context) string {
	if trackID := TrackID(ctx); trackID != "" {
		return trackID
	}
	return NewTrackID()
}
