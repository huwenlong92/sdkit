package tracing

import (
	"context"
	"fmt"
	"net/http"

	"github.com/huwenlong92/sdkit/core/tracecontext"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func Middleware(serviceName string) gin.HandlerFunc {
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	tracer := otel.Tracer(serviceName)

	return func(c *gin.Context) {
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		if trackID := tracking.Get(c); trackID != "" {
			ctx = tracking.WithTrackID(ctx, trackID)
		}

		spanName := c.Request.Method + " " + c.FullPath()
		if c.FullPath() == "" {
			spanName = c.Request.Method + " " + c.Request.URL.Path
		}
		ctx, span := tracer.Start(ctx, spanName,
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			oteltrace.WithAttributes(
				attribute.String("http.request.method", c.Request.Method),
				attribute.String("url.path", c.Request.URL.Path),
				attribute.String("server.address", c.Request.Host),
				attribute.String("service.name", serviceName),
			),
		)
		setHTTPSpanCorrelationAttributes(ctx)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)
		defer recordPanic(span)

		c.Next()

		setHTTPSpanCorrelationAttributes(c.Request.Context())

		status := c.Writer.Status()
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
		for _, err := range c.Errors {
			if err.Err != nil {
				span.RecordError(err.Err)
				span.SetStatus(codes.Error, err.Err.Error())
			}
		}
	}
}

func setHTTPSpanCorrelationAttributes(ctx context.Context) {
	tracecontext.SetHTTPSpanCorrelationAttributes(ctx, oteltrace.SpanFromContext(ctx))
}

func recordPanic(span oteltrace.Span) {
	if err := recover(); err != nil {
		span.RecordError(fmt.Errorf("panic: %v", err))
		span.SetStatus(codes.Error, "panic")
		panic(err)
	}
}
