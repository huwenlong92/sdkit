package robfigcron

import (
	"context"
	"sync/atomic"
	"testing"

	corecron "github.com/huwenlong92/sdkit/core/crontab"
)

func TestSchedulerReloadRejectsInvalidSpec(t *testing.T) {
	registry := corecron.NewRegistry()
	runner := corecron.NewRunner(corecron.RunnerOptions{Registry: registry})
	scheduler := New(runner)

	err := scheduler.Reload(context.Background(), []corecron.Job{{Name: "bad", Enabled: true, Spec: "bad spec"}})
	if err == nil {
		t.Fatal("expected invalid spec error")
	}
}

func TestSchedulerReloadRegistersRunnableJob(t *testing.T) {
	var count atomic.Int64
	registry := corecron.NewRegistry()
	if err := registry.Register(corecron.Template{
		Name:    "tick",
		Handler: corecron.RunHandlerFromFunc(func(context.Context, corecron.Job) error { count.Add(1); return nil }),
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	runner := corecron.NewRunner(corecron.RunnerOptions{Config: corecron.DefaultConfig(), Registry: registry})
	scheduler := New(runner)
	ctx := context.Background()
	if err := scheduler.Reload(ctx, []corecron.Job{
		{Name: "tick", Source: corecron.SourceBuiltin, Mode: corecron.ModeLocal, Enabled: true, Spec: "@every 1s"},
	}); err != nil {
		t.Fatal(err)
	}
	if scheduler.cron == nil {
		t.Fatal("cron was not initialized")
	}
	entries := scheduler.cron.Entries()
	if len(entries) != 1 {
		t.Fatalf("entry count mismatch: got %d", len(entries))
	}
	entries[0].Job.Run()
	if count.Load() != 1 {
		t.Fatalf("job run count mismatch: got %d", count.Load())
	}

	if err := scheduler.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSchedulerReloadSameJobsKeepsRunningCron(t *testing.T) {
	runner := corecron.NewRunner(corecron.RunnerOptions{Config: corecron.DefaultConfig(), Registry: corecron.NewRegistry()})
	scheduler := New(runner)
	ctx := context.Background()
	jobs := []corecron.Job{
		{Name: "tick", Source: corecron.SourceBuiltin, Mode: corecron.ModeLocal, Enabled: true, Spec: "@every 1m"},
	}

	if err := scheduler.Reload(ctx, jobs); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(ctx); err != nil {
		t.Fatal(err)
	}
	firstCron := scheduler.cron
	if !scheduler.running {
		t.Fatal("scheduler should be running after Start")
	}

	if err := scheduler.Reload(ctx, jobs); err != nil {
		t.Fatal(err)
	}
	if scheduler.cron != firstCron {
		t.Fatal("unchanged reload should keep existing cron instance")
	}
	if !scheduler.running {
		t.Fatal("unchanged reload should keep scheduler running")
	}
}

func TestSchedulerReloadChangedJobsRestartsIfAlreadyRunning(t *testing.T) {
	runner := corecron.NewRunner(corecron.RunnerOptions{Config: corecron.DefaultConfig(), Registry: corecron.NewRegistry()})
	scheduler := New(runner)
	ctx := context.Background()
	if err := scheduler.Reload(ctx, []corecron.Job{
		{Name: "tick", Source: corecron.SourceBuiltin, Mode: corecron.ModeLocal, Enabled: true, Spec: "@every 1m"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(ctx); err != nil {
		t.Fatal(err)
	}
	firstCron := scheduler.cron

	if err := scheduler.Reload(ctx, []corecron.Job{
		{Name: "tick", Source: corecron.SourceBuiltin, Mode: corecron.ModeLocal, Enabled: true, Spec: "@every 2m"},
	}); err != nil {
		t.Fatal(err)
	}
	if scheduler.cron == firstCron {
		t.Fatal("changed reload should replace cron instance")
	}
	if !scheduler.running {
		t.Fatal("changed reload should restart scheduler when it was already running")
	}
	if len(scheduler.cron.Entries()) != 1 {
		t.Fatalf("entry count mismatch: got %d", len(scheduler.cron.Entries()))
	}
}
