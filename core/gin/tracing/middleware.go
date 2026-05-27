package gintracing

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/huwenlong92/sdkit/core/tracing"
	"github.com/huwenlong92/sdkit/core/tracking"
)

const defaultServiceName = "sdkitgo"

func Middleware(serviceName string) gin.HandlerFunc {
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	return func(c *gin.Context) {
		ctx := tracing.ExtractHTTPHeader(c.Request.Context(), c.Request.Header)
		if trackID := tracking.TrackID(c.Request.Context()); trackID != "" {
			ctx = tracking.WithTrackID(ctx, trackID)
		}

		spanName := c.Request.Method + " " + c.FullPath()
		if c.FullPath() == "" {
			spanName = c.Request.Method + " " + c.Request.URL.Path
		}
		ctx, span := tracing.StartSpanWithOptions(ctx, spanName,
			tracing.SpanOptions{TracerName: serviceName, Kind: tracing.SpanKindServer},
			tracing.String("http.request.method", c.Request.Method),
			tracing.String("url.path", c.Request.URL.Path),
			tracing.String("server.address", c.Request.Host),
			tracing.String("service.name", serviceName),
		)
		tracing.SetHTTPSpanCorrelationAttributes(ctx, span)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)
		defer recordPanic(span)

		c.Next()

		tracing.SetHTTPSpanCorrelationAttributes(c.Request.Context(), span)

		status := c.Writer.Status()
		span.SetAttributes(tracing.Int("http.response.status_code", status))
		if status >= http.StatusInternalServerError {
			span.SetStatus(tracing.StatusError, http.StatusText(status))
		}
		for _, err := range c.Errors {
			if err.Err != nil {
				span.RecordError(err.Err)
				span.SetStatus(tracing.StatusError, err.Err.Error())
			}
		}
	}
}

func recordPanic(span tracing.Span) {
	if err := recover(); err != nil {
		span.RecordError(fmt.Errorf("panic: %v", err))
		span.SetStatus(tracing.StatusError, "panic")
		panic(err)
	}
}
