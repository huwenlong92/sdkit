package sandbox

import (
	"context"
	"errors"
	"time"
)

type Language string

const (
	LanguagePython Language = "python"
	LanguageGo     Language = "golang"
	LanguageCPP    Language = "cpp"
)

const (
	defaultWorkingDir       = "/workspace"
	defaultTimeout          = 5 * time.Second
	defaultCleanupTimeout   = 10 * time.Second
	defaultLogLimitBytes    = 4 << 20
	defaultMaxFileBytes     = 1 << 20
	defaultMaxTotalFileSize = 8 << 20
	defaultMaxFiles         = 64
	defaultPidsLimit        = int64(128)
	defaultMemoryBytes      = int64(256 << 20)
	defaultNanoCPUs         = int64(1_000_000_000)
	defaultUser             = "65534:65534"
)

var (
	ErrInvalidRequest  = errors.New("sandbox: invalid request")
	ErrImageNotAllowed = errors.New("sandbox: image not allowed")
	ErrImagePull       = errors.New("sandbox: image pull failed")
	ErrCompileFailed   = errors.New("sandbox: compile failed")
	ErrRuntimeFailed   = errors.New("sandbox: runtime failed")
	ErrTimeout         = errors.New("sandbox: timeout")
	ErrResourceLimit   = errors.New("sandbox: resource limit exceeded")
	ErrCleanup         = errors.New("sandbox: cleanup failed")
)

type Sandbox interface {
	Run(ctx context.Context, req *RunRequest) (*RunResult, error)
}

type File struct {
	Path    string
	Content []byte
	Mode    uint32
}

type RegistryAuth struct {
	ServerAddress string
	Username      string
	Password      string
	IdentityToken string
}

type PullPolicy string

const (
	PullNever        PullPolicy = "never"
	PullIfNotPresent PullPolicy = "if-not-present"
	PullAlways       PullPolicy = "always"
)

type RunRequest struct {
	SubmissionID    string
	Language        Language
	Image           string
	Cmd             []string
	CompileCmd      []string
	RunCmd          []string
	Env             []string
	Files           []File
	Timeout         time.Duration
	CompileTimeout  time.Duration
	RunTimeout      time.Duration
	CPUNano         int64
	MemoryBytes     int64
	PidsLimit       int64
	Ulimits         []Ulimit
	WorkingDir      string
	Stdin           []byte
	NetworkDisabled *bool
	ReadonlyRootfs  *bool
	User            string
	RegistryAuth    *RegistryAuth
	PullPolicy      PullPolicy
}

type Ulimit struct {
	Name string
	Soft int64
	Hard int64
}

type RunResult struct {
	ExitCode        int
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
	Duration        time.Duration
	MemoryUsed      uint64
	CPUUsed         float64
	TimedOut        bool
	ContainerID     string
	Phase           string
}

type Options struct {
	Backend           Backend
	TempDir           string
	CleanupTimeout    time.Duration
	LogLimitBytes     int64
	MaxFileBytes      int64
	MaxTotalFileSize  int64
	MaxFiles          int
	AllowedImages     []string
	DefaultPullPolicy PullPolicy
	Metrics           MetricsRecorder
}

type Backend interface {
	Run(ctx context.Context, spec *RunSpec) (*RunResult, error)
}

type RunSpec struct {
	Request           *RunRequest
	WorkspaceDir      string
	CleanupTimeout    time.Duration
	LogLimitBytes     int64
	MaxFileBytes      int64
	MaxTotalFileSize  int64
	MaxFiles          int
	AllowedImages     []string
	DefaultPullPolicy PullPolicy
	Metrics           MetricsRecorder
}

type MetricsRecorder interface {
	RecordRun(ctx context.Context, result *RunResult, err error)
	RecordDuration(ctx context.Context, duration time.Duration)
	RecordTimeout(ctx context.Context)
	RecordMemoryUsage(ctx context.Context, bytes uint64)
	RecordCPUUsage(ctx context.Context, cpu float64)
}

type noopMetricsRecorder struct{}

func NoopMetrics() MetricsRecorder {
	return noopMetricsRecorder{}
}

func (noopMetricsRecorder) RecordRun(context.Context, *RunResult, error)  {}
func (noopMetricsRecorder) RecordDuration(context.Context, time.Duration) {}
func (noopMetricsRecorder) RecordTimeout(context.Context)                 {}
func (noopMetricsRecorder) RecordMemoryUsage(context.Context, uint64)     {}
func (noopMetricsRecorder) RecordCPUUsage(context.Context, float64)       {}

func boolPtr(v bool) *bool {
	return &v
}
