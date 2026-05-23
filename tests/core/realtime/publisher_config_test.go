package realtime_test

import (
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/realtime"
)

func TestValidatePublisherConfigRequiresDriverAndTopic(t *testing.T) {
	if err := realtime.ValidatePublisherConfig(realtime.PublisherConfig{Topic: realtime.DefaultTopic}); err == nil {
		t.Fatal("expected missing driver error")
	}
	if err := realtime.ValidatePublisherConfig(realtime.PublisherConfig{Driver: "memory"}); err == nil {
		t.Fatal("expected missing topic error")
	}
}

func TestValidatePublisherConfigRejectsUnknownDriver(t *testing.T) {
	err := realtime.ValidatePublisherConfig(realtime.PublisherConfig{
		Driver: "bad",
		Topic:  realtime.DefaultTopic,
	})
	if err == nil {
		t.Fatal("expected invalid driver error")
	}
}

func TestValidatePublisherConfigAcceptsConfiguredMemory(t *testing.T) {
	err := realtime.ValidatePublisherConfig(realtime.PublisherConfig{
		Driver: "memory",
		Topic:  realtime.DefaultTopic,
	})
	if err != nil {
		t.Fatalf("ValidatePublisherConfig() error = %v", err)
	}
}

func TestNewConfiguredPublisherNoLongerBuildsCoreDriver(t *testing.T) {
	_, _, err := realtime.NewConfiguredPublisher(realtime.PublisherConfig{
		Driver: "redis",
		Topic:  realtime.DefaultTopic,
	}, "test-sse-publisher")
	if !errors.Is(err, realtime.ErrNilPublisher) {
		t.Fatalf("error = %v, want ErrNilPublisher", err)
	}
}
