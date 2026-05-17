package realtime

import (
	"fmt"
	"os"
	"strings"
)

type PublisherConfig struct {
	Driver       string `mapstructure:"driver" yaml:"driver"`
	Topic        string `mapstructure:"topic" yaml:"topic"`
	TopicPrefix  string `mapstructure:"topic_prefix" yaml:"topic_prefix"`
	NodeName     string `mapstructure:"node_name" yaml:"node_name"`
	StreamMaxLen int64  `mapstructure:"stream_max_len" yaml:"stream_max_len"`
}

func NewConfiguredPublisher(cfg PublisherConfig, logName string) (Publisher, string, error) {
	if err := ValidatePublisherConfig(cfg); err != nil {
		return nil, "", err
	}
	return nil, "", fmt.Errorf("realtime configured publisher moved to bootstrap/pkg runtime: %w", ErrNilPublisher)
}

func ValidatePublisherConfig(cfg PublisherConfig) error {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		return fmt.Errorf("eventbus.driver is required for realtime/eventbus capability; add configs/eventbus.yaml to imports or configure eventbus.driver")
	}
	if driver != "memory" && driver != "redis" && driver != "redis_stream" {
		return fmt.Errorf("eventbus.driver %q is invalid, want memory, redis, or redis_stream", cfg.Driver)
	}
	if strings.TrimSpace(cfg.Topic) == "" {
		return fmt.Errorf("eventbus.topic is required for realtime/eventbus capability")
	}
	return nil
}

func publisherNodeName(cfg PublisherConfig) string {
	if cfg.NodeName != "" {
		return cfg.NodeName
	}
	hostname, err := os.Hostname()
	if err == nil && hostname != "" {
		return hostname
	}
	return "unknown"
}

func normalizePublisherDriver(driver string) string {
	driver = strings.ToLower(strings.TrimSpace(driver))
	if driver == "" {
		return "memory"
	}
	return driver
}

func ensurePublisherDefaultDriver(requested string, actual string) error {
	requested = normalizePublisherDriver(requested)
	actual = normalizePublisherDriver(actual)
	if requested != actual {
		return fmt.Errorf("eventbus driver mismatch: configured %s, default %s", requested, actual)
	}
	return nil
}
