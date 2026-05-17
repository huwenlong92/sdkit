package queue

import "sort"

type MiddlewareStage int

const (
	// MiddlewareStage 已冻结为 runtime governance 的固定顺序，maintenance mode 下不新增 stage。
	//
	// 新治理能力应优先归入已有 stage，只有改变执行语义时才考虑调整 runtime kernel。
	// RecoverStage 用于捕获后续 middleware 或 handler 的 panic，并转换为 error。
	RecoverStage MiddlewareStage = iota

	// TraceStage 用于在观测和业务逻辑执行前创建或恢复 tracing 上下文。
	TraceStage

	// MetricsStage 用于统计任务次数、成功失败和执行耗时。
	MetricsStage

	// LoggingStage 用于记录任务开始、完成和失败日志。
	LoggingStage

	// TimeoutStage 用于任务级超时控制，通常由 queue.WithTimeout 在注册期自动生成。
	TimeoutStage

	// RateLimitStage 用于在占用 worker 并发和业务锁前进行限流。
	RateLimitStage

	// ConcurrencyStage 用于按任务类型、业务 key、租户等维度限制并发执行。
	ConcurrencyStage

	// LockStage 用于在进入业务逻辑前获取业务锁。
	LockStage

	// RetryStage 用于判断错误是否需要重试，并附加重试延迟。
	RetryStage

	// DeadLetterStage 用于在重试策略完成分类后记录终态失败任务。
	DeadLetterStage

	// BusinessStage 是普通 middleware 的默认阶段，也是业务 handler 的执行边界。
	BusinessStage
)

type RuntimeMiddleware struct {
	Stage      MiddlewareStage
	Middleware Middleware
}

func StageMiddleware(stage MiddlewareStage, middleware Middleware) RuntimeMiddleware {
	return RuntimeMiddleware{
		Stage:      stage,
		Middleware: middleware,
	}
}

func BusinessMiddleware(middleware Middleware) RuntimeMiddleware {
	return StageMiddleware(BusinessStage, middleware)
}

func RuntimeMiddlewares(stage MiddlewareStage, middlewares ...Middleware) []RuntimeMiddleware {
	out := make([]RuntimeMiddleware, 0, len(middlewares))
	for _, middleware := range middlewares {
		if middleware != nil {
			out = append(out, StageMiddleware(stage, middleware))
		}
	}
	return out
}

func ChainRuntime(handler HandlerFunc, middlewares ...RuntimeMiddleware) HandlerFunc {
	wrapped := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		middleware := middlewares[i].Middleware
		if middleware != nil {
			wrapped = middleware(wrapped)
		}
	}
	return wrapped
}

func normalizeRuntimeMiddlewares(middlewares []RuntimeMiddleware) []RuntimeMiddleware {
	out := make([]RuntimeMiddleware, 0, len(middlewares))
	for _, middleware := range middlewares {
		if middleware.Middleware != nil {
			out = append(out, middleware)
		}
	}
	sortRuntimeMiddlewares(out)
	return out
}

func cloneRuntimeMiddlewares(in []RuntimeMiddleware) []RuntimeMiddleware {
	if len(in) == 0 {
		return nil
	}
	out := make([]RuntimeMiddleware, len(in))
	copy(out, in)
	return out
}

func sortRuntimeMiddlewares(middlewares []RuntimeMiddleware) {
	sort.SliceStable(middlewares, func(i, j int) bool {
		return middlewares[i].Stage < middlewares[j].Stage
	})
}

func hasRuntimeMiddlewareStage(middlewares []RuntimeMiddleware, stage MiddlewareStage) bool {
	for _, middleware := range middlewares {
		if middleware.Middleware != nil && middleware.Stage == stage {
			return true
		}
	}
	return false
}
