package tracing

import (
	"fmt"
	"net/http"

	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/gin-gonic/gin"
)

func Middleware(serviceName string) gin.HandlerFunc {
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	return func(c *gin.Context) {
		ctx := ExtractHTTPHeader(c.Request.Context(), c.Request.Header)
		if trackID := tracking.Get(c); trackID != "" {
			ctx = tracking.WithTrackID(ctx, trackID)
		}

		spanName := c.Request.Method + " " + c.FullPath()
		if c.FullPath() == "" {
			spanName = c.Request.Method + " " + c.Request.URL.Path
		}
		ctx, span := StartSpanWithOptions(ctx, spanName,
			SpanOptions{TracerName: serviceName, Kind: SpanKindServer},
			String("http.request.method", c.Request.Method),
			String("url.path", c.Request.URL.Path),
			String("server.address", c.Request.Host),
			String("service.name", serviceName),
		)
		SetHTTPSpanCorrelationAttributes(ctx, span)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)
		defer recordPanic(span)

		c.Next()

		SetHTTPSpanCorrelationAttributes(c.Request.Context(), span)

		status := c.Writer.Status()
		span.SetAttributes(Int("http.response.status_code", status))
		if status >= http.StatusInternalServerError {
			span.SetStatus(StatusError, http.StatusText(status))
		}
		for _, err := range c.Errors {
			if err.Err != nil {
				span.RecordError(err.Err)
				span.SetStatus(StatusError, err.Err.Error())
			}
		}
	}
}

func recordPanic(span Span) {
	if err := recover(); err != nil {
		span.RecordError(fmt.Errorf("panic: %v", err))
		span.SetStatus(StatusError, "panic")
		panic(err)
	}
}
