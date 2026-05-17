package robfigcron

import (
	"context"
	"fmt"
	"strings"
	"sync"

	corecron "github.com/huwenlong92/sdkit/core/crontab"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	mu        sync.Mutex
	cron      *cron.Cron
	runner    *corecron.Runner
	running   bool
	signature string
}

func New(runner *corecron.Runner) *Scheduler {
	return &Scheduler{runner: runner}
}

func (s *Scheduler) Name() string {
	return "robfig"
}

func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cron != nil && !s.running {
		s.cron.Start()
		s.running = true
	}
	return nil
}

func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cron != nil && s.running {
		stopCtx := s.cron.Stop()
		s.running = false
		<-stopCtx.Done()
	}
	return nil
}

func (s *Scheduler) Reload(ctx context.Context, jobs []corecron.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	signature := jobsSignature(jobs)
	if s.cron != nil && signature == s.signature {
		return nil
	}

	parser := cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	c := cron.New(cron.WithParser(parser))

	for _, job := range jobs {
		j := job
		if !j.Enabled || j.Spec == "" {
			continue
		}
		if _, err := c.AddFunc(j.Spec, func() {
			s.runner.Run(ctx, j)
		}); err != nil {
			return err
		}
	}

	wasRunning := s.running
	if s.cron != nil && s.running {
		stopCtx := s.cron.Stop()
		s.running = false
		<-stopCtx.Done()
	}

	s.cron = c
	s.signature = signature
	if wasRunning {
		s.cron.Start()
		s.running = true
	}
	return nil
}

func jobsSignature(jobs []corecron.Job) string {
	var b strings.Builder
	for _, job := range jobs {
		if !job.Enabled || job.Spec == "" {
			continue
		}
		_, _ = fmt.Fprintf(
			&b,
			"%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\n",
			job.ID,
			job.Name,
			job.Spec,
			job.Source,
			job.Mode,
			job.Queue,
			job.TaskType,
			job.LockKey,
			job.Timeout,
		)
	}
	return b.String()
}
