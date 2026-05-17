package crontab

import (
	"context"
	"testing"
	"time"
)

func TestRegistryRegisterValidation(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register(Template{}); err == nil {
		t.Fatal("expected missing name error")
	}
	if err := registry.Register(Template{Key: "job", Name: "Job", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Template{Key: "job", Name: "Duplicate", Enabled: true}); err == nil {
		t.Fatal("expected duplicate template error")
	}

	tpl, ok := registry.Get("job")
	if !ok {
		t.Fatal("template not found")
	}
	if tpl.Key != "job" || tpl.Name != "Job" {
		t.Fatalf("template mismatch: %#v", tpl)
	}
}

func TestRegistryListAndBuiltinJobs(t *testing.T) {
	registry := NewRegistry()
	templates := []Template{
		{Name: "z_disabled", Enabled: false},
		{Name: "b_dynamic", Enabled: true},
		{
			Key:     "a_builtin",
			Name:    "Builtin",
			Spec:    "@every 1m",
			Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }),
			Enabled: true,
			Timeout: 3 * time.Second,
		},
	}
	for _, tpl := range templates {
		if err := registry.Register(tpl); err != nil {
			t.Fatal(err)
		}
	}
	list := registry.List()
	if len(list) != 3 || list[0].Key != "a_builtin" || list[2].Key != "z_disabled" {
		t.Fatalf("templates not sorted by name: %#v", list)
	}

	jobs := registry.BuiltinJobs()
	if len(jobs) != 1 {
		t.Fatalf("builtin count mismatch: got %d", len(jobs))
	}
	job := jobs[0]
	if job.Name != "a_builtin" || job.Source != SourceBuiltin || job.Mode != ModeLocal || job.Timeout != 3*time.Second {
		t.Fatalf("unexpected builtin job: %#v", job)
	}
	if job.Spec != "@every 1m" {
		t.Fatalf("unexpected builtin schedule data: %#v", job)
	}
}

func TestRegistryBuiltinJobsUseTemplateSpec(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Key:     "cleanup",
		Name:    "Cleanup",
		Spec:    "@every 1m",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }),
		Enabled: true,
		Timeout: 5 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	jobs := registry.BuiltinJobs()
	if len(jobs) != 1 {
		t.Fatalf("builtin count mismatch: got %d", len(jobs))
	}
	job := jobs[0]
	if job.ID != "builtin.cleanup" || job.Name != "cleanup" || job.Spec != "@every 1m" {
		t.Fatalf("unexpected builtin job: %#v", job)
	}
	if job.Timeout != 5*time.Second {
		t.Fatalf("builtin timeout mismatch: got %s", job.Timeout)
	}
}

func TestRegistryBuiltinJobsSkipTemplatesWithoutSpec(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Name:    "cleanup",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }),
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	if len(registry.BuiltinJobs()) != 0 {
		t.Fatalf("builtin count mismatch: %#v", registry.BuiltinJobs())
	}
}

func TestRegistryRegisterAllStopsOnDuplicate(t *testing.T) {
	registry := NewRegistry()
	err := registry.RegisterAll(
		Template{Key: "job", Name: "Job", Enabled: true},
		Template{Key: "job", Name: "Duplicate", Enabled: true},
	)
	if err == nil {
		t.Fatal("expected duplicate template error")
	}
}

func TestRegistryValidateJob(t *testing.T) {
	handler := RunHandlerFromFunc(func(context.Context, Job) error { return nil })
	tests := []struct {
		name string
		tpl  Template
		job  Job
		want error
	}{
		{name: "missing", job: Job{Name: "missing"}, want: ErrTemplateNotFound},
		{name: "disabled", tpl: Template{Name: "disabled", Handler: handler, Enabled: false, AllowDB: true}, job: Job{Name: "disabled", Source: SourceDynamic, Mode: ModeLocal}, want: ErrTemplateDisabled},
		{name: "dynamic denied", tpl: Template{Name: "dynamic_denied", Handler: handler, Enabled: true, AllowDB: false}, job: Job{Name: "dynamic_denied", Source: SourceDynamic, Mode: ModeLocal}, want: ErrDynamicNotAllowed},
		{name: "local handler missing", tpl: Template{Name: "handler_missing", Enabled: true, AllowDB: true}, job: Job{Name: "handler_missing", Source: SourceDynamic, Mode: ModeLocal}, want: ErrLocalHandlerMissing},
		{name: "ok", tpl: Template{Name: "ok", Handler: handler, Enabled: true, AllowDB: true}, job: Job{Name: "ok", Source: SourceDynamic, Mode: ModeLocal}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			if tt.tpl.Name != "" {
				if err := registry.Register(tt.tpl); err != nil {
					t.Fatal(err)
				}
			}
			_, err := registry.ValidateJob(tt.job)
			if err != tt.want {
				t.Fatalf("error mismatch: got %v want %v", err, tt.want)
			}
		})
	}
}
