package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/huwenlong92/sdkit/core/sandbox/internal/docker"
	"github.com/huwenlong92/sdkit/core/sandbox/profile"
	"github.com/huwenlong92/sdkit/core/sandbox/tracing"
)

type runtime struct {
	opts Options
}

func New(opts Options) (Sandbox, error) {
	opts = normalizeOptions(opts)
	if opts.Backend == nil {
		backend, err := docker.New(docker.Options{
			AllowedImages: opts.AllowedImages,
		})
		if err != nil {
			return nil, fmt.Errorf("sandbox docker backend: %w", err)
		}
		opts.Backend = dockerAdapter{runtime: backend}
	}
	return &runtime{opts: opts}, nil
}

func NewDocker(opts Options) (Sandbox, error) {
	return New(opts)
}

func (r *runtime) Run(ctx context.Context, req *RunRequest) (*RunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	ctx, span := tracing.StartRun(ctx, req)
	defer span.End()

	normalized, err := normalizeRequest(req, r.opts)
	if err != nil {
		tracing.RecordError(span, err)
		r.opts.Metrics.RecordRun(ctx, nil, err)
		return nil, err
	}
	if err := validateFiles(normalized.Files, r.opts); err != nil {
		tracing.RecordError(span, err)
		r.opts.Metrics.RecordRun(ctx, nil, err)
		return nil, err
	}
	tracing.SetRunRequest(span, normalized)

	if normalized.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, normalized.Timeout)
		defer cancel()
	}

	workspace, err := os.MkdirTemp(r.opts.TempDir, "sdkit-sandbox-*")
	if err != nil {
		err = fmt.Errorf("%w: create workspace: %w", ErrInvalidRequest, err)
		tracing.RecordError(span, err)
		r.opts.Metrics.RecordRun(ctx, nil, err)
		return nil, err
	}
	if err := os.Chmod(workspace, 0777); err != nil {
		_ = os.RemoveAll(workspace)
		err = fmt.Errorf("%w: chmod workspace: %w", ErrInvalidRequest, err)
		tracing.RecordError(span, err)
		r.opts.Metrics.RecordRun(ctx, nil, err)
		return nil, err
	}
	if err := WriteFiles(workspace, normalized.Files); err != nil {
		_ = os.RemoveAll(workspace)
		tracing.RecordError(span, err)
		r.opts.Metrics.RecordRun(ctx, nil, err)
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(workspace)
	}()

	spec := &RunSpec{
		Request:           normalized,
		WorkspaceDir:      workspace,
		CleanupTimeout:    r.opts.CleanupTimeout,
		LogLimitBytes:     r.opts.LogLimitBytes,
		MaxFileBytes:      r.opts.MaxFileBytes,
		MaxTotalFileSize:  r.opts.MaxTotalFileSize,
		MaxFiles:          r.opts.MaxFiles,
		AllowedImages:     r.opts.AllowedImages,
		DefaultPullPolicy: r.opts.DefaultPullPolicy,
		Metrics:           r.opts.Metrics,
	}
	result, err := r.opts.Backend.Run(ctx, spec)
	err = mapBackendError(err)
	if result == nil {
		result = &RunResult{}
	}
	result.Duration = time.Since(start)
	if result.TimedOut {
		r.opts.Metrics.RecordTimeout(ctx)
	}
	r.opts.Metrics.RecordDuration(ctx, result.Duration)
	r.opts.Metrics.RecordMemoryUsage(ctx, result.MemoryUsed)
	r.opts.Metrics.RecordCPUUsage(ctx, result.CPUUsed)
	r.opts.Metrics.RecordRun(ctx, result, err)
	tracing.SetRunResult(span, result)
	if err != nil {
		tracing.RecordError(span, err)
		return result, err
	}
	return result, nil
}

type dockerAdapter struct {
	runtime *docker.Runtime
}

func (a dockerAdapter) Run(ctx context.Context, spec *RunSpec) (*RunResult, error) {
	req := spec.Request
	res, err := a.runtime.Run(ctx, &docker.Spec{
		SubmissionID:    req.SubmissionID,
		Language:        string(req.Language),
		Image:           req.Image,
		CompileCmd:      req.CompileCmd,
		RunCmd:          req.RunCmd,
		Env:             req.Env,
		Timeout:         req.Timeout,
		CompileTimeout:  req.CompileTimeout,
		RunTimeout:      req.RunTimeout,
		CPUNano:         req.CPUNano,
		MemoryBytes:     req.MemoryBytes,
		PidsLimit:       req.PidsLimit,
		Ulimits:         convertUlimits(req.Ulimits),
		WorkingDir:      req.WorkingDir,
		Stdin:           req.Stdin,
		NetworkDisabled: req.NetworkDisabled != nil && *req.NetworkDisabled,
		ReadonlyRootfs:  req.ReadonlyRootfs != nil && *req.ReadonlyRootfs,
		User:            req.User,
		RegistryAuth:    convertRegistryAuth(req.RegistryAuth),
		PullPolicy:      string(req.PullPolicy),
		WorkspaceDir:    spec.WorkspaceDir,
		CleanupTimeout:  spec.CleanupTimeout,
		LogLimitBytes:   spec.LogLimitBytes,
	})
	if res == nil {
		return nil, err
	}
	return &RunResult{
		ExitCode:        res.ExitCode,
		Stdout:          res.Stdout,
		Stderr:          res.Stderr,
		StdoutTruncated: res.StdoutTruncated,
		StderrTruncated: res.StderrTruncated,
		MemoryUsed:      res.MemoryUsed,
		CPUUsed:         res.CPUUsed,
		TimedOut:        res.TimedOut,
		ContainerID:     res.ContainerID,
		Phase:           res.Phase,
	}, err
}

func convertUlimits(in []Ulimit) []docker.Ulimit {
	out := make([]docker.Ulimit, 0, len(in))
	for _, item := range in {
		out = append(out, docker.Ulimit{Name: item.Name, Soft: item.Soft, Hard: item.Hard})
	}
	return out
}

func convertRegistryAuth(in *RegistryAuth) *docker.RegistryAuth {
	if in == nil {
		return nil
	}
	return &docker.RegistryAuth{
		ServerAddress: in.ServerAddress,
		Username:      in.Username,
		Password:      in.Password,
		IdentityToken: in.IdentityToken,
	}
}

func mapBackendError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, docker.ErrImageNotAllowed):
		return fmt.Errorf("%w: %w", ErrImageNotAllowed, err)
	case errors.Is(err, docker.ErrImagePull):
		return fmt.Errorf("%w: %w", ErrImagePull, err)
	case errors.Is(err, docker.ErrCompileFailed):
		return fmt.Errorf("%w: %w", ErrCompileFailed, err)
	case errors.Is(err, docker.ErrTimeout):
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	case errors.Is(err, docker.ErrCleanup):
		return fmt.Errorf("%w: %w", ErrCleanup, err)
	case errors.Is(err, docker.ErrRuntimeFailed):
		return fmt.Errorf("%w: %w", ErrRuntimeFailed, err)
	default:
		return err
	}
}

func normalizeOptions(opts Options) Options {
	if opts.CleanupTimeout <= 0 {
		opts.CleanupTimeout = defaultCleanupTimeout
	}
	if opts.LogLimitBytes <= 0 {
		opts.LogLimitBytes = defaultLogLimitBytes
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = defaultMaxFileBytes
	}
	if opts.MaxTotalFileSize <= 0 {
		opts.MaxTotalFileSize = defaultMaxTotalFileSize
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = defaultMaxFiles
	}
	if opts.DefaultPullPolicy == "" {
		opts.DefaultPullPolicy = PullIfNotPresent
	}
	if opts.Metrics == nil {
		opts.Metrics = NoopMetrics()
	}
	return opts
}

func normalizeRequest(req *RunRequest, opts Options) (*RunRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request required", ErrInvalidRequest)
	}
	out := *req
	if out.Language != "" {
		applied, err := profile.Apply(profile.Request{
			Language:   string(out.Language),
			Image:      out.Image,
			CompileCmd: out.CompileCmd,
			RunCmd:     firstNonEmpty(out.RunCmd, out.Cmd),
			WorkingDir: out.WorkingDir,
			Env:        out.Env,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
		}
		out.Image = applied.Image
		out.CompileCmd = applied.CompileCmd
		out.RunCmd = applied.RunCmd
		out.WorkingDir = applied.WorkingDir
		out.Env = applied.Env
	}
	if out.Image == "" {
		return nil, fmt.Errorf("%w: image required", ErrInvalidRequest)
	}
	if len(out.RunCmd) == 0 && len(out.Cmd) > 0 {
		out.RunCmd = out.Cmd
	}
	if len(out.RunCmd) == 0 {
		return nil, fmt.Errorf("%w: command required", ErrInvalidRequest)
	}
	if out.WorkingDir == "" {
		out.WorkingDir = defaultWorkingDir
	}
	if out.Timeout <= 0 {
		out.Timeout = defaultTimeout
	}
	if out.CPUNano <= 0 {
		out.CPUNano = defaultNanoCPUs
	}
	if out.MemoryBytes <= 0 {
		out.MemoryBytes = defaultMemoryBytes
	}
	if out.PidsLimit <= 0 {
		out.PidsLimit = defaultPidsLimit
	}
	if out.NetworkDisabled == nil {
		out.NetworkDisabled = boolPtr(true)
	}
	if out.ReadonlyRootfs == nil {
		out.ReadonlyRootfs = boolPtr(true)
	}
	if out.User == "" {
		out.User = defaultUser
	}
	if out.PullPolicy == "" {
		out.PullPolicy = opts.DefaultPullPolicy
	}
	return &out, nil
}

func firstNonEmpty(primary, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}
