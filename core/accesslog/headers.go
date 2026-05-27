package accesslog

import (
	"net/http"
	"strings"

	"github.com/huwenlong92/sdkit/core/jsonx"
)

var defaultSensitiveHeaders = []string{
	"authorization",
	"cookie",
	"set-cookie",
	"x-api-key",
	"x-auth-token",
}

// FilterHeaders filters sensitive request headers and returns a JSON string.
func FilterHeaders(h http.Header) string {
	return filterHeaders(h, defaultSensitiveHeaders)
}

// FilterHeadersWithSensitiveHeaders filters the specified sensitive headers.
func FilterHeadersWithSensitiveHeaders(h http.Header, headers ...string) string {
	return filterHeaders(h, normalizeSensitiveValues(headers...))
}

// FilterHeadersWithAdditionalSensitiveHeaders filters default sensitive headers plus custom headers.
func FilterHeadersWithAdditionalSensitiveHeaders(h http.Header, headers ...string) string {
	return filterHeaders(h, appendSensitiveValues(defaultSensitiveHeaders, headers...))
}

func FilterHeadersWithList(h http.Header, sensitiveHeaders []string) string {
	if len(h) == 0 {
		return ""
	}

	sensitive := make(map[string]struct{}, len(sensitiveHeaders))
	for _, header := range sensitiveHeaders {
		header = strings.ToLower(strings.TrimSpace(header))
		if header == "" {
			continue
		}
		sensitive[header] = struct{}{}
	}

	result := make(map[string]interface{}, len(h))
	for k, v := range h {
		lk := strings.ToLower(k)
		if _, ok := sensitive[lk]; ok {
			continue
		}
		if len(v) == 1 {
			result[k] = v[0]
		} else {
			result[k] = v
		}
	}

	b, err := jsonx.Marshal(result)
	if err != nil {
		return ""
	}
	return string(b)
}

func filterHeaders(h http.Header, sensitiveHeaders []string) string {
	return FilterHeadersWithList(h, sensitiveHeaders)
}
