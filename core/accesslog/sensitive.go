package accesslog

import "strings"

func AppendSensitiveValues(base []string, values ...string) []string {
	return appendSensitiveValues(base, values...)
}

func NormalizeSensitiveValues(values ...string) []string {
	return appendSensitiveValues(nil, values...)
}

func normalizeSensitiveValues(values ...string) []string {
	return NormalizeSensitiveValues(values...)
}

func DefaultSensitiveHeaders() []string {
	return appendSensitiveValues(nil, defaultSensitiveHeaders...)
}

func appendSensitiveValues(base []string, values ...string) []string {
	out := make([]string, 0, len(base)+len(values))
	seen := make(map[string]struct{}, len(base)+len(values))
	for _, value := range base {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
