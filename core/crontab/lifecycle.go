package crontab

import (
	"context"
	"sync"
	"time"
)

type FailureReport struct {
	EntryID     string
	TemplateKey string
	StartedAt   time.Time
	FinishedAt  time.Time
	Duration    time.Duration
	TraceID     string
	Error       error
}

type FailureHandler func(ctx context.Context, report FailureReport)

var failureHandlers struct {
	mu       sync.RWMutex
	handlers []FailureHandler
}

func UseFailureHandler(handlers ...FailureHandler) {
	failureHandlers.mu.Lock()
	defer failureHandlers.mu.Unlock()
	failureHandlers.handlers = append([]FailureHandler(nil), handlers...)
}

func runFailureHandlers(ctx context.Context, report FailureReport) {
	failureHandlers.mu.RLock()
	handlers := append([]FailureHandler(nil), failureHandlers.handlers...)
	failureHandlers.mu.RUnlock()

	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			handler(ctx, report)
		}()
	}
}
