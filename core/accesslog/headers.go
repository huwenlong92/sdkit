package accesslog

import (
	"net/http"
	"strings"

	"github.com/huwenlong92/sdkit/core/jsonx"
)

var sensitiveHeaders = map[string]struct{}{
	"authorization": {},
	"cookie":        {},
	"set-cookie":    {},
	"x-api-key":     {},
	"x-auth-token":  {},
}

// FilterHeaders filters sensitive request headers and returns a JSON string.
func FilterHeaders(h http.Header) string {
	if len(h) == 0 {
		return ""
	}

	result := make(map[string]interface{}, len(h))
	for k, v := range h {
		lk := strings.ToLower(k)
		if _, ok := sensitiveHeaders[lk]; ok {
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
