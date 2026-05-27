package tracecontext

import (
	"context"
	"strings"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracking"
)

const traceparentHeader = "traceparent"

type Carrier interface {
	Get(string) string
	Set(string, string)
}

type KeyCarrier interface {
	Carrier
	Keys() []string
}

type MapCarrier map[string]string

func (c MapCarrier) Get(key string) string {
	return HeaderValue(c, key)
}

func (c MapCarrier) Set(key string, value string) {
	if c == nil || key == "" || value == "" {
		return
	}
	c[key] = value
}

func (c MapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}
	return keys
}

func HeadersFromContext(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	headers := map[string]string{}
	InjectCarrier(ctx, MapCarrier(headers))
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func ContextFromHeaders(ctx context.Context, headers map[string]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(headers) == 0 {
		return ctx
	}
	return ExtractCarrier(ctx, MapCarrier(headers))
}

func InjectCarrier(ctx context.Context, carrier Carrier) {
	if ctx == nil || carrier == nil {
		return
	}
	if traceparent := Traceparent(ctx); traceparent != "" {
		carrier.Set(traceparentHeader, traceparent)
	}
	if traceID := TraceID(ctx); traceID != "" {
		carrier.Set(logger.TraceIDKey, traceID)
	}
	if spanID := SpanID(ctx); spanID != "" {
		carrier.Set(logger.SpanIDKey, spanID)
	}
	if trackID := tracking.TrackID(ctx); trackID != "" {
		carrier.Set(tracking.Header, trackID)
	}
	if requestID := RequestID(ctx); requestID != "" {
		carrier.Set(requestid.Header, requestID)
	}
}

func ExtractCarrier(ctx context.Context, carrier Carrier) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if carrier == nil {
		return ctx
	}
	if trackID := carrierValue(carrier, tracking.Header); trackID != "" {
		ctx = tracking.WithTrackID(ctx, trackID)
	}
	if requestID := carrierValue(carrier, requestid.Header); requestID != "" {
		ctx = requestid.WithRequestID(ctx, requestID)
	}
	headers := carrierFields(carrier)
	traceID := TraceIDFromHeaders(headers)
	spanID := SpanIDFromHeaders(headers)
	if traceID != "" || spanID != "" {
		ctx = ContextWithTrace(ctx, traceID, spanID)
	}
	if traceID != "" {
		ctx = logger.WithField(ctx, logger.TraceIDKey, traceID)
	}
	if spanID != "" {
		ctx = logger.WithField(ctx, logger.SpanIDKey, spanID)
	}
	return ctx
}

func RequestID(ctx context.Context) string {
	return requestid.FromContext(ctx)
}

func Traceparent(ctx context.Context) string {
	traceID := TraceID(ctx)
	spanID := SpanID(ctx)
	if traceID == "" || spanID == "" {
		return ""
	}
	return "00-" + traceID + "-" + spanID + "-01"
}

func HeaderValue(headers map[string]string, key string) string {
	if len(headers) == 0 || key == "" {
		return ""
	}
	if v := headers[key]; v != "" {
		return v
	}
	for k, v := range headers {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func TraceIDFromHeaders(headers map[string]string) string {
	if traceID := HeaderValue(headers, logger.TraceIDKey); traceID != "" {
		return traceID
	}
	parts := strings.Split(HeaderValue(headers, traceparentHeader), "-")
	if len(parts) >= 4 && len(parts[1]) == 32 {
		return parts[1]
	}
	return ""
}

func SpanIDFromHeaders(headers map[string]string) string {
	if spanID := HeaderValue(headers, logger.SpanIDKey); spanID != "" {
		return spanID
	}
	parts := strings.Split(HeaderValue(headers, traceparentHeader), "-")
	if len(parts) >= 4 && len(parts[2]) == 16 {
		return parts[2]
	}
	return ""
}

func carrierValue(carrier Carrier, key string) string {
	if carrier == nil || key == "" {
		return ""
	}
	if value := carrier.Get(key); value != "" {
		return value
	}
	keyCarrier, ok := carrier.(KeyCarrier)
	if !ok {
		return ""
	}
	for _, field := range keyCarrier.Keys() {
		if strings.EqualFold(field, key) {
			return carrier.Get(field)
		}
	}
	return ""
}

func carrierFields(carrier Carrier) map[string]string {
	keyCarrier, ok := carrier.(KeyCarrier)
	if !ok {
		fields := []string{traceparentHeader, logger.TraceIDKey, logger.SpanIDKey, tracking.Header, requestid.Header}
		headers := make(map[string]string, len(fields))
		for _, field := range fields {
			if value := carrier.Get(field); value != "" {
				headers[field] = value
			}
		}
		return headers
	}
	fields := keyCarrier.Keys()
	if len(fields) == 0 {
		return nil
	}
	headers := make(map[string]string, len(fields))
	for _, field := range fields {
		if value := carrier.Get(field); value != "" {
			headers[field] = value
		}
	}
	return headers
}
