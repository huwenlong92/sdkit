package queue

import "time"

type RuntimeMetadata struct {
	Name           string
	Service        string
	Driver         string
	Worker         string
	Queues         map[string]int
	DefaultQueue   string
	StrictPriority bool
	Concurrency    int
	RetryCount     int
	MaxRetry       *int
	Timeout        time.Duration
	Delay          time.Duration
	Priority       int
	Trace          TraceMetadata
	WorkerMetadata WorkerMetadata
	Middleware     MiddlewareMetadata
	RateLimit      RateLimitConfig
}

type TraceMetadata struct {
	Enabled   bool
	TrackID   string
	RequestID string
	TraceID   string
	SpanID    string
}

type WorkerMetadata struct {
	ID             string
	Name           string
	Service        string
	Queues         map[string]int
	StrictPriority bool
	Concurrency    int
}

type MiddlewareMetadata struct {
	Count int
	Names []string
}

type RegistryMetadata struct {
	Handlers   []HandlerMetadata
	Middleware MiddlewareMetadata
}

func RuntimeMetadataFromConfig(name string, service string, cfg Config) RuntimeMetadata {
	normalized := cfg.Normalize()
	metadata := RuntimeMetadata{
		Name:           name,
		Service:        service,
		Driver:         normalized.Driver,
		Queues:         cloneQueueWeights(normalized.Queues),
		DefaultQueue:   DefaultQueueName,
		StrictPriority: normalized.StrictPriority,
		Concurrency:    normalized.Concurrency,
		Trace:          TraceMetadata{Enabled: true},
		RateLimit:      normalized.RateLimit,
	}
	metadata.WorkerMetadata = WorkerMetadata{
		Name:           metadata.Worker,
		Service:        service,
		Queues:         cloneQueueWeights(metadata.Queues),
		StrictPriority: metadata.StrictPriority,
		Concurrency:    metadata.Concurrency,
	}
	return metadata
}

func RuntimeMetadataFromWorkerProfile(name string, service string, cfg Config, profile WorkerProfile) RuntimeMetadata {
	metadata := RuntimeMetadataFromConfig(name, service, cfg)
	metadata.Worker = profile.Name
	metadata.Queues = cloneQueueWeights(profile.Queues)
	metadata.StrictPriority = profile.StrictPriority
	metadata.Concurrency = profile.Concurrency
	metadata.WorkerMetadata = WorkerMetadata{
		Name:           profile.Name,
		Service:        service,
		Queues:         cloneQueueWeights(profile.Queues),
		StrictPriority: profile.StrictPriority,
		Concurrency:    profile.Concurrency,
	}
	return metadata
}

func cloneRuntimeMetadata(metadata RuntimeMetadata) RuntimeMetadata {
	metadata.Queues = cloneQueueWeights(metadata.Queues)
	metadata.WorkerMetadata.Queues = cloneQueueWeights(metadata.WorkerMetadata.Queues)
	metadata.Middleware.Names = cloneStrings(metadata.Middleware.Names)
	return metadata
}

func cloneQueueWeights(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneIntPtr(in *int) *int {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
