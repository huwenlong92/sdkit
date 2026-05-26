package asynq

import (
	"context"

	"github.com/huwenlong92/sdkit/core/queue"
)

func (q *Queue) Run(ctx context.Context) error {
	if q == nil || q.server == nil || q.mux == nil {
		return queue.ErrNotInitialized
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- q.server.Run(q.mux)
	}()
	select {
	case <-ctx.Done():
		q.server.Shutdown()
		return nil
	case err := <-errCh:
		return err
	}
}

func (q *Queue) Shutdown(ctx context.Context) error {
	if q == nil || q.server == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		q.server.Shutdown()
		close(done)
	}()
	if ctx == nil {
		<-done
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		q.server.Stop()
		return ctx.Err()
	}
}

func (q *Queue) Close() error {
	if q == nil {
		return nil
	}
	_ = q.Shutdown(context.Background())
	if q.client != nil {
		if err := q.client.Close(); err != nil {
			return err
		}
	}
	if q.inspector != nil {
		return q.inspector.Close()
	}
	return nil
}
