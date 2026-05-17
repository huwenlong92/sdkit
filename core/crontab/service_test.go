package crontab

import (
	"context"
	"testing"
)

func TestValidateSpecAndNextRuns(t *testing.T) {
	if err := ValidateSpec("@every 1m"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSpec("bad spec"); err == nil {
		t.Fatal("expected invalid spec error")
	}
	runs, err := NextRuns("@every 1m", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || !runs[1].After(runs[0]) {
		t.Fatalf("unexpected next runs: %#v", runs)
	}
}

func TestMemoryLogStreamerFilters(t *testing.T) {
	streamer := NewMemoryLogStreamer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := streamer.Subscribe(ctx, LogFilter{RunID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := streamer.Publish(context.Background(), LogEvent{RunID: "run-2", Message: "skip"}); err != nil {
		t.Fatal(err)
	}
	if err := streamer.Publish(context.Background(), LogEvent{RunID: "run-1", Message: "keep"}); err != nil {
		t.Fatal(err)
	}
	got := <-ch
	if got.Message != "keep" {
		t.Fatalf("unexpected event: %#v", got)
	}
}

func TestServiceTemplateFilteringAndRunOnceValidation(t *testing.T) {
	registry := NewRegistry()
	called := false
	manager, err := NewManager(ManagerOptions{
		Config:    DefaultConfig(),
		Registry:  registry,
		Store:     NoopStore{},
		Scheduler: &fakeScheduler{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Template{
		Key:     "disabled",
		Name:    "Disabled",
		Enabled: false,
		AllowDB: true,
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			return nil
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Template{
		Key:     "manual",
		Name:    "Manual",
		Enabled: true,
		AllowDB: false,
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			called = true
			return nil
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Template{
		Key:     "db",
		Name:    "DB",
		Enabled: true,
		AllowDB: true,
		Handler: RunHandlerFromFunc(func(ctx context.Context, job Job) error {
			return nil
		}),
	}); err != nil {
		t.Fatal(err)
	}

	templates, err := manager.ListDBTemplates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 || templates[0].Key != "db" {
		t.Fatalf("unexpected db templates: %#v", templates)
	}
	if err := manager.RunOnce(context.Background(), RunOnceRequest{TemplateKey: "manual", Payload: "ok"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("manual template was not executed")
	}
}
