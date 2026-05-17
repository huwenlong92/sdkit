package runtime

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func NotifySignals() (<-chan os.Signal, func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	return signals, func() {
		signal.Stop(signals)
	}
}

func StopOnSignal(ctx context.Context, app *App, signals <-chan os.Signal) <-chan error {
	errCh := make(chan error, 1)
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		select {
		case <-ctx.Done():
			errCh <- nil
		case _, ok := <-signals:
			if !ok {
				errCh <- nil
				return
			}
			errCh <- app.Stop(context.Background())
		}
	}()
	return errCh
}

func (a *App) watchSignals(ctx context.Context) func() {
	signals, stopSignals := NotifySignals()
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-ctx.Done():
		case _, ok := <-signals:
			if ok {
				_ = a.Stop(context.Background())
			}
		}
	}()
	return func() {
		close(done)
		stopSignals()
	}
}
