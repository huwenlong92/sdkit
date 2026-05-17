package tracing

import "context"

func ShutdownNoop(ctx context.Context) error {
	return nil
}
