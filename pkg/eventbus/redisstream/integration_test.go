package redisstream

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/huwenlong92/sdkit/core/eventbus"

	goredis "github.com/redis/go-redis/v9"
)

func TestIntegrationRedisStreamRoundTrip(t *testing.T) {
	client := integrationRedisClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	prefix := fmt.Sprintf("sdkitgo:test:eventbus:stream:%d", time.Now().UnixNano())
	topic := "rt:events"
	streamKey := prefix + ":" + topic
	t.Cleanup(func() { _ = client.Del(context.Background(), streamKey).Err() })

	bus := New(client, prefix, "integration-group", "integration-consumer", WithBlockTime(100*time.Millisecond), WithMaxLen(100))
	t.Cleanup(func() { _ = bus.Close() })

	received := make(chan *eventbus.Event, 1)
	subscription, err := bus.Subscribe(ctx, topic, func(_ context.Context, ev *eventbus.Event) error {
		received <- ev
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	t.Cleanup(func() { _ = subscription.Close() })

	published, err := eventbus.NewJSONEvent(ctx, topic, map[string]string{"source": "redis-stream"}, map[string]string{eventbus.HeaderTraceID: "trace-real-stream"})
	if err != nil {
		t.Fatalf("NewJSONEvent: %v", err)
	}
	if err := bus.Publish(ctx, published); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case ev := <-received:
		if ev.Topic != topic {
			t.Fatalf("topic = %q, want %q", ev.Topic, topic)
		}
		if ev.Headers[eventbus.HeaderTraceID] != "trace-real-stream" {
			t.Fatalf("trace_id = %q", ev.Headers[eventbus.HeaderTraceID])
		}
		var payload map[string]string
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatalf("payload json: %v", err)
		}
		if payload["source"] != "redis-stream" {
			t.Fatalf("payload source = %q", payload["source"])
		}
	case <-ctx.Done():
		t.Fatalf("wait redis stream event: %v", ctx.Err())
	}
}

func integrationRedisClient(t *testing.T) *goredis.Client {
	t.Helper()
	if os.Getenv("SDKITGO_INTEGRATION") != "1" {
		t.Skip("set SDKITGO_INTEGRATION=1 to run real Redis EventBus integration test")
	}
	addr := os.Getenv("SDKITGO_REDIS_ADDR")
	if addr == "" {
		t.Skip("set SDKITGO_REDIS_ADDR to run real Redis EventBus integration test")
	}
	db := 0
	if raw := os.Getenv("SDKITGO_REDIS_DB"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("SDKITGO_REDIS_DB: %v", err)
		}
		db = n
	}
	client := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Username: os.Getenv("SDKITGO_REDIS_USERNAME"),
		Password: os.Getenv("SDKITGO_REDIS_PASSWORD"),
		DB:       db,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Fatalf("redis ping: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}
