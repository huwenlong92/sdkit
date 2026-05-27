package accesslog

import (
	"net/http"

	coreaccesslog "github.com/huwenlong92/sdkit/core/accesslog"
)

type Actor = coreaccesslog.Actor
type Config = coreaccesslog.Config
type Entry = coreaccesslog.Entry
type Logger = coreaccesslog.Logger
type Writer = coreaccesslog.Writer

func NewLogger(writer Writer, cfg Config) *Logger {
	return coreaccesslog.NewLogger(writer, cfg)
}

func FilterHeaders(h http.Header) string {
	return coreaccesslog.FilterHeaders(h)
}

func FilterHeadersWithSensitiveHeaders(h http.Header, headers ...string) string {
	return coreaccesslog.FilterHeadersWithSensitiveHeaders(h, headers...)
}

func FilterHeadersWithAdditionalSensitiveHeaders(h http.Header, headers ...string) string {
	return coreaccesslog.FilterHeadersWithAdditionalSensitiveHeaders(h, headers...)
}
