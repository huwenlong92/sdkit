package eventbus

import (
	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/requestid"
	"github.com/huwenlong92/sdkit/core/tracking"
)

const (
	HeaderTraceparent  = "traceparent"
	HeaderTracestate   = "tracestate"
	HeaderBaggage      = "baggage"
	HeaderTraceID      = logger.TraceIDKey
	HeaderSpanID       = logger.SpanIDKey
	HeaderTrackID      = tracking.Header
	HeaderRequestID    = requestid.Header
	HeaderConnectionID = "connection_id"
	HeaderSessionID    = "session_id"
)

var eventFlowHeaderKeys = []string{
	HeaderTraceparent,
	HeaderTracestate,
	HeaderBaggage,
	HeaderTraceID,
	HeaderSpanID,
	HeaderTrackID,
	HeaderRequestID,
	HeaderConnectionID,
	HeaderSessionID,
}

// EventFlowHeaderKeys returns the correlation headers recognized by EventFlow.
//
// EventBus does not own these field semantics. Trace/span/request/track values
// are injected and extracted through core/tracing, core/tracking and
// core/requestid helpers. Connection/session values are carried only as
// headers and are intentionally not modeled as Event top-level fields.
func EventFlowHeaderKeys() []string {
	return append([]string(nil), eventFlowHeaderKeys...)
}
