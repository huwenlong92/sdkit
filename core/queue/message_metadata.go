package queue

import "time"

const (
	MessageMetadataPattern        = "queue.pattern"
	MessageMetadataQueue          = "queue.queue"
	MessageMetadataMaxRetry       = "queue.max_retry"
	MessageMetadataTimeout        = "queue.timeout"
	MessageMetadataDelay          = "queue.delay"
	MessageMetadataPriority       = "queue.priority"
	MessageMetadataTrace          = "queue.trace"
	MessageMetadataLockKey        = "queue.lock.key"
	MessageMetadataLockTTL        = "queue.lock.ttl"
	MessageMetadataWorker         = "queue.worker"
	MessageMetadataConcurrencyKey = "queue.concurrency.key"
)

func SetMessageMetadata(msg *Message, key string, value any) {
	if msg == nil || key == "" || value == nil {
		return
	}
	if msg.Metadata == nil {
		msg.Metadata = map[string]any{}
	}
	msg.Metadata[key] = value
}

func MessageMetadataValue(msg *Message, key string) (any, bool) {
	if msg == nil || key == "" || len(msg.Metadata) == 0 {
		return nil, false
	}
	value, ok := msg.Metadata[key]
	return value, ok
}

func MessageMetadataDuration(msg *Message, key string) (time.Duration, bool) {
	value, ok := MessageMetadataValue(msg, key)
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case time.Duration:
		return v, true
	case int:
		return time.Duration(v), true
	case int64:
		return time.Duration(v), true
	default:
		return 0, false
	}
}

func MessageMetadataString(msg *Message, key string) (string, bool) {
	value, ok := MessageMetadataValue(msg, key)
	if !ok {
		return "", false
	}
	out, ok := value.(string)
	return out, ok && out != ""
}
