package realtime_test

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/realtime"
	eventbuspublisher "github.com/huwenlong92/sdkit/pkg/realtime/publisher/eventbus"
)

var (
	_ realtime.Publisher = eventbuspublisher.New(nil, realtime.DefaultTopic)
	_ realtime.Service   = eventbuspublisher.New(nil, realtime.DefaultTopic)
)

func TestNewOptionsNormalizesDefaults(t *testing.T) {
	options := realtime.NewOptions()
	if options.ClientBufferSize != 64 {
		t.Fatalf("client buffer size: want 64, got %d", options.ClientBufferSize)
	}
	if options.Logger == nil {
		t.Fatal("logger should be normalized")
	}
}

func TestOptionsApplyOverrides(t *testing.T) {
	log := warningLogger{}
	options := realtime.NewOptions(realtime.WithClientBufferSize(1), realtime.WithLogger(log))
	if options.ClientBufferSize != 1 {
		t.Fatalf("client buffer size: want 1, got %d", options.ClientBufferSize)
	}
	if options.Logger == nil {
		t.Fatal("custom logger should be retained")
	}
}

func TestDefaultConfigProducesNormalizedOptions(t *testing.T) {
	cfg := realtime.DefaultConfig()
	if cfg.Topic != realtime.DefaultTopic {
		t.Fatalf("default topic: want %s, got %s", realtime.DefaultTopic, cfg.Topic)
	}
	options := cfg.Options()
	if options.ClientBufferSize != 64 {
		t.Fatalf("client buffer size: want 64, got %d", options.ClientBufferSize)
	}
}

func TestPublisherInterfaceIsTransportFree(t *testing.T) {
	var publisher realtime.Publisher = eventbuspublisher.New(nil, realtime.DefaultTopic)
	err := publisher.Broadcast(context.Background(), realtime.NewEvent("notify", map[string]string{"ok": "1"}))
	if err == nil {
		t.Fatal("default publisher should return error when eventbus default is not initialized")
	}
}

func TestTopLevelPushAPIsRequireDefaultEventBus(t *testing.T) {
	if err := realtime.PushUser(context.Background(), "1001", "notify", nil); err == nil {
		t.Fatal("PushUser should return error when eventbus default is not initialized")
	}
	if err := realtime.PushRoom(context.Background(), "room-1", "notify", nil); err == nil {
		t.Fatal("PushRoom should return error when eventbus default is not initialized")
	}
	if err := realtime.Broadcast(context.Background(), "notify", nil); err == nil {
		t.Fatal("Broadcast should return error when eventbus default is not initialized")
	}
}

type warningLogger struct {
}

func (l warningLogger) Warn(_ string, fields ...any) {
}
