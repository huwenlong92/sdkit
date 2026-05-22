package queue

type Capability string

type Driver interface {
	Name() string
	Capabilities() map[Capability]bool
	Supports(cap Capability) bool

	NewClient(cfg Config) (Client, error)
	NewWorker(cfg Config, profile WorkerProfile) (Worker, error)
	NewManager(cfg Config) (Manager, error)
}

const (
	CapEnqueue     Capability = "enqueue"
	CapConsume     Capability = "consume"
	CapRetry       Capability = "retry"
	CapTimeout     Capability = "timeout"
	CapDeadline    Capability = "deadline"
	CapDelay       Capability = "delay"
	CapUnique      Capability = "unique"
	CapPriority    Capability = "priority"
	CapRateLimit   Capability = "rate_limit"
	CapCancel      Capability = "cancel"
	CapPauseResume Capability = "pause_resume"
	CapDLQ         Capability = "dlq"
	CapInspector   Capability = "inspector"
	CapBatch       Capability = "batch"
	CapChain       Capability = "chain"
	CapProgress    Capability = "progress"
	CapHeartbeat   Capability = "heartbeat"
	CapLock        Capability = "lock"
	CapIdempotency Capability = "idempotency"
	CapMetrics     Capability = "metrics"
	CapLog         Capability = "log"
	CapTrace       Capability = "trace"
	CapActionLog   Capability = "action_log"
	CapOutbox      Capability = "outbox"
)

func CloneCapabilities(in map[Capability]bool) map[Capability]bool {
	out := make(map[Capability]bool, len(in))
	for cap, ok := range in {
		out[cap] = ok
	}
	return out
}
