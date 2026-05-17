package middleware

import "github.com/huwenlong92/sdkit/core/queue"

func Staged(stage queue.MiddlewareStage, middleware queue.Middleware) queue.RuntimeMiddleware {
	return queue.StageMiddleware(stage, middleware)
}

func RecoverStage() queue.RuntimeMiddleware {
	return Staged(queue.RecoverStage, Recover())
}

func TracingStage() queue.RuntimeMiddleware {
	return Staged(queue.TraceStage, Tracing())
}

func MetricsStage(recorder queue.MetricsRecorder) queue.RuntimeMiddleware {
	return Staged(queue.MetricsStage, Metrics(recorder))
}

func LoggingStage() queue.RuntimeMiddleware {
	return Staged(queue.LoggingStage, Logging())
}

func TimeoutStage() queue.RuntimeMiddleware {
	return Staged(queue.TimeoutStage, Timeout())
}

func RateLimitStage(limiter queue.RateLimiter, keyFn RateLimitKeyFunc) queue.RuntimeMiddleware {
	return Staged(queue.RateLimitStage, RateLimit(limiter, keyFn))
}

func ConcurrencyStage(limiter queue.ConcurrencyLimiter, keyFns ...ConcurrencyKeyFunc) queue.RuntimeMiddleware {
	return Staged(queue.ConcurrencyStage, Concurrency(limiter, keyFns...))
}

func LockStage(locker queue.Locker, keyFns ...LockKeyFunc) queue.RuntimeMiddleware {
	return Staged(queue.LockStage, Lock(locker, keyFns...))
}

func RetryStage(strategy queue.RetryStrategy) queue.RuntimeMiddleware {
	return Staged(queue.RetryStage, Retry(strategy))
}

func DeadLetterStage(deadletter queue.DeadLetter) queue.RuntimeMiddleware {
	return Staged(queue.DeadLetterStage, DeadLetter(deadletter))
}
