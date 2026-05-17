package crontab

import (
	"context"
	"errors"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/tracking"

	"github.com/google/uuid"
)

type RunHandler func(*RunContext) RunResult

type RunResult struct {
	Status     Status
	Err        error
	PanicStack string
}

type RunContext struct {
	ctx        context.Context
	runner     *Runner
	Job        Job
	Template   Template
	Entry      Entry
	RunID      string
	StartedAt  time.Time
	InstanceID string
	Result     RunResult
}

func newRunContext(ctx context.Context, runner *Runner, job Job) *RunContext {
	if ctx == nil {
		ctx = context.Background()
	}
	runID := uuid.NewString()
	ctx = logger.WithField(ctx, logger.RunIDKey, runID)
	if job.ID != "" {
		ctx = logger.WithField(ctx, logger.JobIDKey, job.ID)
	}
	instanceID := ""
	if runner != nil {
		instanceID = runner.cfg.InstanceID
	}
	return &RunContext{
		ctx:        ctx,
		runner:     runner,
		Job:        job,
		Entry:      JobToEntry(job),
		RunID:      runID,
		StartedAt:  time.Now(),
		InstanceID: instanceID,
	}
}

func (c *RunContext) Context() context.Context {
	if c == nil || c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *RunContext) SetContext(ctx context.Context) {
	if c == nil || ctx == nil {
		return
	}
	c.ctx = ctx
}

func (c *RunContext) clone() *RunContext {
	if c == nil {
		return nil
	}
	cloned := *c
	return &cloned
}

func (c *RunContext) runningLog() RunLog {
	return c.runLog(StatusRunning, nil, "", time.Time{})
}

func (c *RunContext) finishedLog(result RunResult, finishedAt time.Time) RunLog {
	return c.runLog(result.Status, result.Err, result.PanicStack, finishedAt)
}

func (c *RunContext) runLog(status Status, err error, panicStack string, finishedAt time.Time) RunLog {
	if c == nil {
		return RunLog{}
	}
	log := RunLog{
		JobID:      c.Job.ID,
		JobName:    c.Job.Name,
		RunID:      c.RunID,
		Source:     c.Job.Source,
		Mode:       c.Job.Mode,
		Spec:       c.Job.Spec,
		Queue:      c.Job.Queue,
		TaskType:   c.Job.TaskType,
		Payload:    c.Job.Payload,
		TrackID:    tracking.TrackID(c.Context()),
		Status:     status,
		StartedAt:  c.StartedAt.Unix(),
		Host:       hostname(),
		InstanceID: c.InstanceID,
		PanicStack: panicStack,
	}
	if err != nil {
		log.Error = err.Error()
	}
	if !finishedAt.IsZero() {
		log.FinishedAt = finishedAt.Unix()
		log.DurationMs = finishedAt.Sub(c.StartedAt).Milliseconds()
	}
	return log
}

func (r RunResult) normalize() RunResult {
	if r.Status != "" {
		return r
	}
	if r.Err != nil {
		r.Status = StatusFailed
		return r
	}
	r.Status = StatusSuccess
	return r
}

func RunHandlerFromFunc(handler func(context.Context, Job) error) RunHandler {
	return func(c *RunContext) RunResult {
		if handler == nil {
			return RunResult{Status: StatusFailed, Err: ErrLocalHandlerMissing}
		}
		if c == nil {
			if err := handler(context.Background(), Job{}); err != nil {
				return runResultFromError(err)
			}
			return RunResult{Status: StatusSuccess}
		}
		ctx := ContextWithEntry(c.Context(), c.Entry)
		if err := handler(ctx, c.Job); err != nil {
			return runResultFromError(err)
		}
		return RunResult{Status: StatusSuccess}
	}
}

func validateStatus(err error) Status {
	switch {
	case errors.Is(err, ErrTemplateNotFound):
		return StatusTemplateMissing
	case errors.Is(err, ErrTemplateDisabled):
		return StatusTemplateDisabled
	case errors.Is(err, ErrDynamicNotAllowed):
		return StatusSkipped
	default:
		return StatusFailed
	}
}
