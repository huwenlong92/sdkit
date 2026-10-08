package sysprobe

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var ErrUnavailable = errors.New("sysprobe: service is unavailable")

type Config struct {
	Host            HostConfig            `mapstructure:"host" yaml:"host"`
	NetworkCapacity NetworkCapacityConfig `mapstructure:"network_capacity" yaml:"network_capacity"`
	DependencyTTL   time.Duration         `mapstructure:"dependency_ttl" yaml:"dependency_ttl"`
	CheckTimeout    time.Duration         `mapstructure:"check_timeout" yaml:"check_timeout"`
	Dependencies    []DependencySpec      `mapstructure:"dependencies" yaml:"dependencies"`
}

type NetworkCapacityConfig struct {
	ReceiveLimitMbps float64 `json:"receive_limit_mbps" mapstructure:"receive_limit_mbps" yaml:"receive_limit_mbps"`
	SendLimitMbps    float64 `json:"send_limit_mbps" mapstructure:"send_limit_mbps" yaml:"send_limit_mbps"`
}

type Snapshot struct {
	At                time.Time             `json:"at"`
	Ready             bool                  `json:"ready"`
	Host              HostSnapshot          `json:"host"`
	NetworkCapacity   NetworkCapacityConfig `json:"network_capacity"`
	Dependencies      []DependencyStatus    `json:"dependencies"`
	DependencyChecked time.Time             `json:"dependency_checked_at"`
}

// HostSampler permits an explicitly supplied host collector.
type HostSampler interface {
	Snapshot(context.Context) (HostSnapshot, error)
}

// Runner must stop when its context is canceled.
type Runner interface {
	Run(context.Context, DependencySpec) ([]byte, error)
}

type Option func(*Service)

func WithHostSampler(host HostSampler) Option {
	return func(s *Service) {
		if host != nil {
			s.host = host
		}
	}
}

func WithRunner(runner Runner) Option {
	return func(s *Service) {
		if runner != nil {
			s.runner = runner
		}
	}
}

type Service struct {
	host     HostSampler
	config   Config
	runner   Runner
	closed   atomic.Bool
	lifetime context.Context
	cancel   context.CancelFunc

	dependencyGate chan struct{}
	dependencies   []DependencyStatus
	checkedAt      time.Time
}

func New(config Config, options ...Option) *Service {
	if config.DependencyTTL <= 0 {
		config.DependencyTTL = 30 * time.Second
	}
	if config.CheckTimeout <= 0 || config.CheckTimeout > 10*time.Second {
		config.CheckTimeout = 2 * time.Second
	}
	if config.NetworkCapacity.ReceiveLimitMbps < 0 {
		config.NetworkCapacity.ReceiveLimitMbps = 0
	}
	if config.NetworkCapacity.SendLimitMbps < 0 {
		config.NetworkCapacity.SendLimitMbps = 0
	}
	config.Dependencies = append([]DependencySpec(nil), config.Dependencies...)
	for i := range config.Dependencies {
		config.Dependencies[i].Args = append([]string(nil), config.Dependencies[i].Args...)
		config.Dependencies[i].Env = append([]string(nil), config.Dependencies[i].Env...)
	}
	lifetime, cancel := context.WithCancel(context.Background())
	service := &Service{host: NewHost(config.Host), config: config, runner: execRunner{}, lifetime: lifetime, cancel: cancel, dependencyGate: make(chan struct{}, 1)}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	if s == nil || s.lifetime == nil || s.closed.Load() {
		return Snapshot{}, ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.lifetime, cancel)
	defer func() { stop(); cancel() }()
	host, err := safeHostSnapshot(ctx, s.host)
	if err != nil {
		return Snapshot{}, err
	}
	dependencies, checkedAt := s.dependencySnapshot(ctx)
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	ready := true
	for _, status := range dependencies {
		if status.Required && !status.Compatible {
			ready = false
			break
		}
	}
	return Snapshot{
		At: time.Now(), Ready: ready, Host: host,
		NetworkCapacity: s.config.NetworkCapacity,
		Dependencies:    dependencies, DependencyChecked: checkedAt,
	}, nil
}

// Close makes subsequent snapshots unavailable. The service owns no permanent goroutines.
func (s *Service) Close() error {
	if s != nil {
		s.closed.Store(true)
		if s.cancel != nil {
			s.cancel()
		}
	}
	return nil
}

func safeHostSnapshot(ctx context.Context, host HostSampler) (snapshot HostSnapshot, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			snapshot = HostSnapshot{
				CollectedAt: time.Now(), Disks: []Disk{},
				Issues: []Issue{{Component: "sysprobe", Message: "host collection failed"}},
			}
			err = nil
		}
	}()
	if host == nil {
		return HostSnapshot{}, fmt.Errorf("sysprobe: host sampler is unavailable")
	}
	return host.Snapshot(ctx)
}

func (s *Service) dependencySnapshot(ctx context.Context) ([]DependencyStatus, time.Time) {
	select {
	case s.dependencyGate <- struct{}{}:
	case <-ctx.Done():
		return nil, time.Time{}
	}
	defer func() { <-s.dependencyGate }()
	if ctx.Err() != nil {
		return nil, time.Time{}
	}
	if !s.checkedAt.IsZero() && time.Since(s.checkedAt) < s.config.DependencyTTL {
		return append([]DependencyStatus(nil), s.dependencies...), s.checkedAt
	}
	statuses := make([]DependencyStatus, len(s.config.Dependencies))
	semaphore := make(chan struct{}, 8)
	var workers sync.WaitGroup
	for i, spec := range s.config.Dependencies {
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			workers.Wait()
			return nil, time.Time{}
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-semaphore }()
			statuses[i] = s.checkDependency(ctx, spec)
		}()
	}
	workers.Wait()
	if ctx.Err() != nil {
		return nil, time.Time{}
	}
	s.dependencies, s.checkedAt = statuses, time.Now()
	return append([]DependencyStatus(nil), statuses...), s.checkedAt
}
