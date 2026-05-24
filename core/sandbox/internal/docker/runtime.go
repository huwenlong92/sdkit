package docker

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/sandbox/tracing"

	"github.com/moby/moby/client"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

var (
	ErrImageNotAllowed = errors.New("docker sandbox: image not allowed")
	ErrImagePull       = errors.New("docker sandbox: image pull failed")
	ErrCompileFailed   = errors.New("docker sandbox: compile failed")
	ErrRuntimeFailed   = errors.New("docker sandbox: runtime failed")
	ErrTimeout         = errors.New("docker sandbox: timeout")
	ErrCleanup         = errors.New("docker sandbox: cleanup failed")
)

type Runtime struct {
	client        *client.Client
	allowedImages map[string]struct{}
	log           *zap.Logger
}

func New(opts Options) (*Runtime, error) {
	clientOpts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if opts.Host != "" {
		clientOpts = append(clientOpts, client.WithHost(opts.Host))
	}
	cli, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	allowed := make(map[string]struct{}, len(opts.AllowedImages))
	for _, image := range opts.AllowedImages {
		if image != "" {
			allowed[image] = struct{}{}
		}
	}
	return &Runtime{
		client:        cli,
		allowedImages: allowed,
		log:           logger.Named("sandbox"),
	}, nil
}

func (r *Runtime) Run(ctx context.Context, spec *Spec) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if spec == nil {
		return nil, fmt.Errorf("docker sandbox: spec required")
	}
	if err := r.ensureImage(ctx, spec); err != nil {
		return nil, err
	}

	if len(spec.CompileCmd) > 0 {
		compileCtx := ctx
		if spec.CompileTimeout > 0 {
			var cancel context.CancelFunc
			compileCtx, cancel = context.WithTimeout(ctx, spec.CompileTimeout)
			defer cancel()
		}
		res, err := r.runContainer(compileCtx, spec, "compile", spec.CompileCmd, nil)
		if err != nil {
			cleanupErr := r.cleanupWorkspace(spec)
			if cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("%w: %w", ErrCleanup, cleanupErr))
			}
			return &res.Result, err
		}
		if res.ExitCode != 0 {
			err := fmt.Errorf("%w: exit_code=%d", ErrCompileFailed, res.ExitCode)
			cleanupErr := r.cleanupWorkspace(spec)
			if cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("%w: %w", ErrCleanup, cleanupErr))
			}
			return &res.Result, err
		}
	}

	runCtx := ctx
	if spec.RunTimeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, spec.RunTimeout)
		defer cancel()
	}
	res, err := r.runContainer(runCtx, spec, "run", spec.RunCmd, spec.Stdin)
	if err != nil {
		cleanupErr := r.cleanupWorkspace(spec)
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("%w: %w", ErrCleanup, cleanupErr))
		}
		return &res.Result, err
	}
	if res.ExitCode != 0 {
		err = fmt.Errorf("%w: exit_code=%d", ErrRuntimeFailed, res.ExitCode)
	}
	cleanupErr := r.cleanupWorkspace(spec)
	if cleanupErr != nil {
		err = errors.Join(err, fmt.Errorf("%w: %w", ErrCleanup, cleanupErr))
	}
	return &res.Result, err
}

func (r *Runtime) cleanupWorkspace(spec *Spec) error {
	if spec == nil || spec.WorkspaceDir == "" {
		return nil
	}
	if err := os.RemoveAll(spec.WorkspaceDir); err != nil {
		return fmt.Errorf("remove workspace: %w", err)
	}
	return nil
}

func (r *Runtime) runContainer(ctx context.Context, spec *Spec, phase string, cmd []string, stdin []byte) (*containerRunResult, error) {
	ctx, span := tracing.StartStep(ctx, "sandbox."+phase,
		attribute.String("image", spec.Image),
		attribute.String("submission.id", spec.SubmissionID),
		attribute.String("phase", phase),
	)
	defer span.End()

	created, err := r.create(ctx, spec, phase, cmd, stdin)
	if err != nil {
		tracing.RecordError(span, err)
		return &containerRunResult{Result: Result{ExitCode: -1, Phase: phase}}, err
	}
	result := &containerRunResult{
		containerID: created.ID,
		Result: Result{
			ExitCode:    -1,
			ContainerID: created.ID,
			Phase:       phase,
		},
	}
	tracing.SetContainerID(span, created.ID)
	log := logger.WithContext(ctx, r.log).With(
		zap.String("container_id", created.ID),
		zap.String("submission_id", spec.SubmissionID),
		zap.String("phase", phase),
	)
	log.Info("sandbox container created")

	defer func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), spec.CleanupTimeout)
		defer cancelCleanup()
		if err := r.remove(cleanupCtx, created.ID); err != nil {
			log.Warn("sandbox container cleanup failed", zap.Error(err))
		}
	}()

	if err := r.start(ctx, created.ID); err != nil {
		tracing.RecordError(span, err)
		return result, err
	}
	log.Info("sandbox container started")
	if len(stdin) > 0 {
		if err := r.attachStdin(ctx, created.ID, stdin); err != nil {
			tracing.RecordError(span, err)
			return result, err
		}
	}

	statsCtx, cancelStats := context.WithCancel(ctx)
	statsCh := r.collectStats(statsCtx, created.ID)
	waitRes, waitErr := r.wait(ctx, created.ID)
	cancelStats()
	stats := <-statsCh
	result.MemoryUsed = stats.MemoryUsed
	result.CPUUsed = stats.CPUUsed

	if waitErr != nil {
		if ctx.Err() != nil {
			result.TimedOut = true
			killCtx, cancelKill := context.WithTimeout(context.Background(), spec.CleanupTimeout)
			_ = r.kill(killCtx, created.ID)
			cancelKill()
			waitErr = fmt.Errorf("%w: %w", ErrTimeout, ctx.Err())
		}
		tracing.RecordError(span, waitErr)
	}
	if waitRes != nil {
		result.ExitCode = int(waitRes.StatusCode)
	}

	logsCtx, cancelLogs := context.WithTimeout(context.Background(), spec.CleanupTimeout)
	stdout, stderr, stdoutTruncated, stderrTruncated, logsErr := r.logs(logsCtx, created.ID, spec.LogLimitBytes)
	cancelLogs()
	result.Stdout = stdout
	result.Stderr = stderr
	result.StdoutTruncated = stdoutTruncated
	result.StderrTruncated = stderrTruncated
	if logsErr != nil && waitErr == nil {
		waitErr = logsErr
	}
	tracing.SetRunResult(span, &result.Result)
	log.Info("sandbox container finished",
		zap.Int("exit_code", result.ExitCode),
		zap.Bool("timed_out", result.TimedOut),
		zap.Uint64("memory_used", result.MemoryUsed),
		zap.Float64("cpu_used", result.CPUUsed),
	)
	return result, waitErr
}
