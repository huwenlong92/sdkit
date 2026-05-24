package docker

import "time"

const (
	PullNever        = "never"
	PullIfNotPresent = "if-not-present"
	PullAlways       = "always"
)

type Options struct {
	Host          string
	AllowedImages []string
}

type Spec struct {
	SubmissionID    string
	Language        string
	Image           string
	CompileCmd      []string
	RunCmd          []string
	Env             []string
	Timeout         time.Duration
	CompileTimeout  time.Duration
	RunTimeout      time.Duration
	CPUNano         int64
	MemoryBytes     int64
	PidsLimit       int64
	Ulimits         []Ulimit
	WorkingDir      string
	Stdin           []byte
	NetworkDisabled bool
	ReadonlyRootfs  bool
	User            string
	RegistryAuth    *RegistryAuth
	PullPolicy      string
	WorkspaceDir    string
	CleanupTimeout  time.Duration
	LogLimitBytes   int64
}

type RegistryAuth struct {
	ServerAddress string
	Username      string
	Password      string
	IdentityToken string
}

type Ulimit struct {
	Name string
	Soft int64
	Hard int64
}

type Result struct {
	ExitCode        int
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
	MemoryUsed      uint64
	CPUUsed         float64
	TimedOut        bool
	ContainerID     string
	Phase           string
}

type containerRunResult struct {
	Result
	containerID string
}
