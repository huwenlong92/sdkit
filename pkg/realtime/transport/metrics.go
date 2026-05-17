package transport

import "time"

type Metrics interface {
	Inc(name string, labels ...string)
	Observe(name string, value time.Duration, labels ...string)
}

type NopMetrics struct{}

func (NopMetrics) Inc(string, ...string) {}

func (NopMetrics) Observe(string, time.Duration, ...string) {}

func NormalizeMetrics(metrics Metrics) Metrics {
	if metrics == nil {
		return NopMetrics{}
	}
	return metrics
}
