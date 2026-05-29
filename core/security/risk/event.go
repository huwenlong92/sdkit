package risk

import (
	"strconv"
	"strings"
)

func normalizeEvent(event Event) Event {
	event.Service = firstNonEmpty(strings.TrimSpace(event.Service), "admin")
	event.Scene = strings.TrimSpace(event.Scene)
	event.Event = strings.TrimSpace(event.Event)
	event.Level = strings.TrimSpace(event.Level)
	event.SubjectType = strings.TrimSpace(event.SubjectType)
	event.SubjectID = strings.TrimSpace(event.SubjectID)
	event.UID = strings.TrimSpace(event.UID)
	event.IP = strings.TrimSpace(event.IP)
	event.DeviceID = strings.TrimSpace(event.DeviceID)
	event.TrackID = strings.TrimSpace(event.TrackID)
	event.TraceID = strings.TrimSpace(event.TraceID)
	event.RequestID = strings.TrimSpace(event.RequestID)
	event.Action = strings.TrimSpace(event.Action)
	if event.Extra == nil {
		event.Extra = map[string]any{}
	}
	return event
}

func TargetValues(event Event) map[string]string {
	targets := make(map[string]string)
	if event.UID != "" {
		targets["uid"] = event.UID
	}
	if event.IP != "" {
		targets["ip"] = event.IP
	}
	if event.DeviceID != "" {
		targets["device_id"] = event.DeviceID
	}
	if event.SubjectID != "" {
		targets["subject_id"] = event.SubjectID
	}
	for _, key := range []string{"account", "mobile", "email"} {
		if value, ok := event.Extra[key].(string); ok && strings.TrimSpace(value) != "" {
			targets[key] = strings.TrimSpace(value)
		}
	}
	return targets
}

func TargetValue(event Event, targetType string) (string, bool) {
	switch targetType {
	case "uid":
		return event.UID, event.UID != ""
	case "ip":
		return event.IP, event.IP != ""
	case "device_id":
		return event.DeviceID, event.DeviceID != ""
	case "subject_id":
		return event.SubjectID, event.SubjectID != ""
	default:
		value, ok := event.Extra[targetType]
		if !ok {
			return "", false
		}
		switch v := value.(type) {
		case string:
			v = strings.TrimSpace(v)
			return v, v != ""
		case int64:
			return strconv.FormatInt(v, 10), true
		case int:
			return strconv.Itoa(v), true
		case float64:
			if v == float64(int64(v)) {
				return strconv.FormatInt(int64(v), 10), true
			}
		}
	}
	return "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
