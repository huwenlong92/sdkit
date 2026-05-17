package queue

import "time"

type Option func(*EnqueueOptions)

type EnqueueOptions struct {
	Queue        string
	TaskID       string
	MaxRetry     *int
	Timeout      time.Duration
	Deadline     time.Time
	ProcessAt    time.Time
	ProcessIn    time.Duration
	UniqueTTL    time.Duration
	Retention    time.Duration
	Group        string
	Priority     int
	RateLimitKey string
	Trace        bool
}

func Queue(name string) Option {
	return func(o *EnqueueOptions) {
		o.Queue = name
	}
}

func WithQueue(name string) Option { return Queue(name) }

func MaxRetry(n int) Option {
	return func(o *EnqueueOptions) {
		if n < 0 {
			n = 0
		}
		o.MaxRetry = &n
	}
}

func WithMaxRetry(n int) Option { return MaxRetry(n) }

func WithRetry(n int) Option { return MaxRetry(n) }

func Timeout(d time.Duration) Option {
	return func(o *EnqueueOptions) {
		o.Timeout = d
	}
}

func WithTimeout(d time.Duration) Option { return Timeout(d) }

func Deadline(t time.Time) Option {
	return func(o *EnqueueOptions) {
		o.Deadline = t
	}
}

func WithDeadline(t time.Time) Option { return Deadline(t) }

func ProcessAt(t time.Time) Option {
	return func(o *EnqueueOptions) {
		o.ProcessAt = t
	}
}

func WithProcessAt(t time.Time) Option { return ProcessAt(t) }

func ProcessIn(d time.Duration) Option {
	return func(o *EnqueueOptions) {
		o.ProcessIn = d
	}
}

func WithProcessIn(d time.Duration) Option { return ProcessIn(d) }

func WithDelay(d time.Duration) Option { return ProcessIn(d) }

func TaskID(id string) Option {
	return func(o *EnqueueOptions) {
		o.TaskID = id
	}
}

func WithTaskID(id string) Option { return TaskID(id) }

func Unique(ttl time.Duration) Option {
	return func(o *EnqueueOptions) {
		o.UniqueTTL = ttl
	}
}

func WithUnique(ttl time.Duration) Option { return Unique(ttl) }

func Retention(d time.Duration) Option {
	return func(o *EnqueueOptions) {
		o.Retention = d
	}
}

func WithRetention(d time.Duration) Option { return Retention(d) }

func Group(name string) Option {
	return func(o *EnqueueOptions) {
		o.Group = name
	}
}

func WithGroup(name string) Option { return Group(name) }

func WithPriority(priority int) Option {
	return func(o *EnqueueOptions) {
		o.Priority = priority
	}
}

func WithRateLimitKey(key string) Option {
	return func(o *EnqueueOptions) {
		o.RateLimitKey = key
	}
}

func WithTrace(enabled bool) Option {
	return func(o *EnqueueOptions) {
		o.Trace = enabled
	}
}

func ApplyOptions(opts []Option) EnqueueOptions {
	out := EnqueueOptions{Queue: DefaultQueueName, Trace: true}
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

func applyOptions(opts []Option) EnqueueOptions {
	return ApplyOptions(opts)
}
