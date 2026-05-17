package redisx

import (
	"testing"

	"github.com/redis/go-redis/v9/maintnotifications"
)

func TestNewDisablesMaintNotifications(t *testing.T) {
	client := New(Config{Addr: "127.0.0.1:6379"})
	defer client.Close()

	if client.Rdb == nil {
		t.Fatal("redis client is nil")
	}
	if got := client.Rdb.Options().MaintNotificationsConfig.Mode; got != maintnotifications.ModeDisabled {
		t.Fatalf("MaintNotificationsConfig.Mode = %q, want %q", got, maintnotifications.ModeDisabled)
	}
}
