package sandbox

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusMetrics struct {
	runTotal     *prometheus.CounterVec
	runDuration  prometheus.Histogram
	timeoutTotal prometheus.Counter
	memoryUsage  prometheus.Gauge
	cpuUsage     prometheus.Gauge
}

func NewPrometheusMetrics(reg prometheus.Registerer) (*PrometheusMetrics, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	m := &PrometheusMetrics{
		runTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: MetricRunTotal,
			Help: "Total sandbox runs.",
		}, []string{"status"}),
		runDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    MetricRunDuration,
			Help:    "Sandbox run duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		timeoutTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: MetricTimeoutTotal,
			Help: "Total sandbox runs killed by timeout.",
		}),
		memoryUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: MetricMemoryUsage,
			Help: "Last observed sandbox memory usage in bytes.",
		}),
		cpuUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: MetricCPUUsage,
			Help: "Last observed sandbox CPU usage in seconds.",
		}),
	}
	for _, collector := range []prometheus.Collector{m.runTotal, m.runDuration, m.timeoutTotal, m.memoryUsage, m.cpuUsage} {
		if err := reg.Register(collector); err != nil {
			if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
				m.useRegistered(collector, already.ExistingCollector)
				continue
			}
			return nil, err
		}
	}
	return m, nil
}

func (m *PrometheusMetrics) useRegistered(newCollector, existing prometheus.Collector) {
	switch newCollector {
	case m.runTotal:
		if c, ok := existing.(*prometheus.CounterVec); ok {
			m.runTotal = c
		}
	case m.runDuration:
		if c, ok := existing.(prometheus.Histogram); ok {
			m.runDuration = c
		}
	case m.timeoutTotal:
		if c, ok := existing.(prometheus.Counter); ok {
			m.timeoutTotal = c
		}
	case m.memoryUsage:
		if c, ok := existing.(prometheus.Gauge); ok {
			m.memoryUsage = c
		}
	case m.cpuUsage:
		if c, ok := existing.(prometheus.Gauge); ok {
			m.cpuUsage = c
		}
	}
}

func (m *PrometheusMetrics) RecordRun(ctx context.Context, result *RunResult, err error) {
	if m == nil || m.runTotal == nil {
		return
	}
	status := "success"
	if err != nil {
		status = "failure"
	}
	if result != nil && result.TimedOut {
		status = "timeout"
	}
	m.runTotal.WithLabelValues(status).Inc()
}

func (m *PrometheusMetrics) RecordDuration(ctx context.Context, duration time.Duration) {
	if m == nil || m.runDuration == nil || duration <= 0 {
		return
	}
	m.runDuration.Observe(duration.Seconds())
}

func (m *PrometheusMetrics) RecordTimeout(context.Context) {
	if m == nil || m.timeoutTotal == nil {
		return
	}
	m.timeoutTotal.Inc()
}

func (m *PrometheusMetrics) RecordMemoryUsage(ctx context.Context, bytes uint64) {
	if m == nil || m.memoryUsage == nil {
		return
	}
	m.memoryUsage.Set(float64(bytes))
}

func (m *PrometheusMetrics) RecordCPUUsage(ctx context.Context, cpu float64) {
	if m == nil || m.cpuUsage == nil {
		return
	}
	m.cpuUsage.Set(cpu)
}
