package crontab

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	jobs      []Job
	statuses  []JobStatus
	logs      []RunLog
	listErr   error
	enabled   []string
	disabled  []string
	running   []RunLog
	finished  []RunLog
	logsLimit int
}

func (s *fakeStore) ListEnabledJobs(ctx context.Context) ([]Job, error) {
	return s.jobs, s.listErr
}

func (s *fakeStore) ListJobs(ctx context.Context) ([]JobStatus, error) {
	return s.statuses, nil
}

func (s *fakeStore) MarkRunning(ctx context.Context, log RunLog) error {
	s.running = append(s.running, log)
	return nil
}

func (s *fakeStore) MarkFinished(ctx context.Context, log RunLog) error {
	s.finished = append(s.finished, log)
	return nil
}

func (s *fakeStore) Enable(ctx context.Context, name string) error {
	s.enabled = append(s.enabled, name)
	return nil
}

func (s *fakeStore) Disable(ctx context.Context, name string) error {
	s.disabled = append(s.disabled, name)
	return nil
}

func (s *fakeStore) Logs(ctx context.Context, name string, limit int) ([]RunLog, error) {
	s.logsLimit = limit
	return s.logs, nil
}

type fakeScheduler struct {
	mu      sync.Mutex
	started bool
	stopped bool
	reloads [][]Job
}

func (s *fakeScheduler) Name() string { return "fake" }

func (s *fakeScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
	return nil
}

func (s *fakeScheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	return nil
}

func (s *fakeScheduler) Reload(ctx context.Context, jobs []Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := append([]Job(nil), jobs...)
	s.reloads = append(s.reloads, snapshot)
	return nil
}

func TestNewManagerRequiresDependencies(t *testing.T) {
	if _, err := NewManager(ManagerOptions{Scheduler: &fakeScheduler{}}); err == nil {
		t.Fatal("expected registry error")
	}
	if _, err := NewManager(ManagerOptions{Registry: NewRegistry()}); err == nil {
		t.Fatal("expected scheduler error")
	}
}

func TestManagerStartLoadsBuiltinAndDynamicJobs(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Key:     "builtin",
		Name:    "Builtin",
		Spec:    "@every 1m",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }),
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Template{
		Key:     "dynamic",
		Name:    "Dynamic",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	store := &fakeStore{jobs: []Job{{ID: "1", Name: "dynamic", Source: SourceDynamic, Spec: "@every 2m", Payload: `{"default":true}`, Enabled: true}}}
	scheduler := &fakeScheduler{}
	manager, err := NewManager(ManagerOptions{Registry: registry, Store: store, Scheduler: scheduler})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !scheduler.started {
		t.Fatal("scheduler was not started")
	}
	if len(scheduler.reloads) != 1 {
		t.Fatalf("reload count mismatch: got %d", len(scheduler.reloads))
	}
	jobs := scheduler.reloads[0]
	if len(jobs) != 2 {
		t.Fatalf("job count mismatch: got %d", len(jobs))
	}
	dynamic := jobs[1]
	if dynamic.Label != "Dynamic" || dynamic.Spec != "@every 2m" || dynamic.Mode != ModeLocal || dynamic.Payload != `{"default":true}` {
		t.Fatalf("dynamic job was not prepared: %#v", dynamic)
	}
}

func TestManagerStartSkipsDBJobWhenTemplateMissing(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Key:     "dynamic",
		Name:    "Dynamic",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	store := &fakeStore{jobs: []Job{
		{ID: "1", Name: "missing", Source: SourceDB, Spec: "@every 1m", Enabled: true},
		{ID: "2", Name: "dynamic", Source: SourceDB, Spec: "@every 1m", Enabled: true},
	}}
	scheduler := &fakeScheduler{}
	manager, err := NewManager(ManagerOptions{Registry: registry, Store: store, Scheduler: scheduler})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !scheduler.started {
		t.Fatal("scheduler was not started")
	}
	if len(scheduler.reloads) != 1 || len(scheduler.reloads[0]) != 1 {
		t.Fatalf("unexpected scheduled jobs: %#v", scheduler.reloads)
	}
	if scheduler.reloads[0][0].Name != "dynamic" {
		t.Fatalf("unexpected scheduled job: %#v", scheduler.reloads[0][0])
	}
	if len(store.finished) != 1 {
		t.Fatalf("missing template job was not marked finished: %#v", store.finished)
	}
	if store.finished[0].JobID != "db.1" || store.finished[0].JobName != "missing" || store.finished[0].Status != StatusTemplateMissing {
		t.Fatalf("unexpected missing template status: %#v", store.finished[0])
	}
}

func TestManagerRunOnceReturnsJobRunningWhenTaskLockHeld(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{
		Name:    "manual",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error { return nil }),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	locker := &captureLockedLocker{}
	manager, err := NewManager(ManagerOptions{
		Config:     DefaultConfig(),
		Registry:   registry,
		Store:      &fakeStore{},
		Repository: &fakeEntryRepository{entry: EntryInfo{ID: "db.1", Name: "Manual", TemplateKey: "manual", Spec: "@every 1m", Source: SourceDB, Enabled: true}},
		Scheduler:  &fakeScheduler{},
		Locker:     locker,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = manager.RunOnce(context.Background(), RunOnceRequest{EntryID: "1"})
	if !errors.Is(err, ErrJobRunning) {
		t.Fatalf("expected ErrJobRunning, got %v", err)
	}
	if locker.key != "crontab:entry:db.1" {
		t.Fatalf("lock key mismatch: got %s", locker.key)
	}
}

func TestManagerRunOnceReturnsHandlerError(t *testing.T) {
	wantErr := errors.New("manual failed")
	registry := NewRegistry()
	if err := registry.Register(Template{
		Name: "manual_failure",
		Handler: RunHandlerFromFunc(func(context.Context, Job) error {
			return wantErr
		}),
		Enabled: true,
		AllowDB: true,
	}); err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(ManagerOptions{
		Config:     DefaultConfig(),
		Registry:   registry,
		Store:      &fakeStore{},
		Repository: &fakeEntryRepository{entry: EntryInfo{ID: "db.2", Name: "Manual failure", TemplateKey: "manual_failure", Spec: "@every 1m", Source: SourceDB, Enabled: true}},
		Scheduler:  &fakeScheduler{},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = manager.RunOnce(context.Background(), RunOnceRequest{EntryID: "2"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected handler error, got %v", err)
	}
}

type fakeEntryRepository struct {
	entry EntryInfo
}

func (r *fakeEntryRepository) ListEnabled(ctx context.Context) ([]Entry, error) {
	return nil, nil
}

func (r *fakeEntryRepository) ListEntries(ctx context.Context) ([]EntryInfo, error) {
	return []EntryInfo{r.entry}, nil
}

func (r *fakeEntryRepository) GetEntry(ctx context.Context, id string) (EntryInfo, error) {
	return r.entry, nil
}

func (r *fakeEntryRepository) Create(ctx context.Context, entry Entry) (EntryInfo, error) {
	return entryInfo(entry), nil
}

func (r *fakeEntryRepository) Update(ctx context.Context, entry Entry) error {
	return nil
}

func (r *fakeEntryRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func (r *fakeEntryRepository) UpdateEnabled(ctx context.Context, id string, enabled bool) error {
	return nil
}

func (r *fakeEntryRepository) ListRuns(ctx context.Context, filter RunFilter) ([]RunLog, error) {
	return nil, nil
}

type captureLockedLocker struct {
	key string
}

func (l *captureLockedLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	l.key = key
	return nil, false, nil
}

func (l *captureLockedLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (Lock, bool, error) {
	l.key = key
	return nil, false, nil
}

func TestManagerStoreMethods(t *testing.T) {
	store := &fakeStore{
		statuses: []JobStatus{{Name: "job"}},
		logs:     []RunLog{{JobName: "job"}},
	}
	manager, err := NewManager(ManagerOptions{Registry: NewRegistry(), Store: store, Scheduler: &fakeScheduler{}})
	if err != nil {
		t.Fatal(err)
	}

	if err := manager.Enable(context.Background(), "job"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Disable(context.Background(), "job"); err != nil {
		t.Fatal(err)
	}
	if got, err := manager.Status(context.Background()); err != nil || len(got) != 1 {
		t.Fatalf("status mismatch: got=%#v err=%v", got, err)
	}
	if got, err := manager.Logs(context.Background(), "job", 7); err != nil || len(got) != 1 || store.logsLimit != 7 {
		t.Fatalf("logs mismatch: got=%#v limit=%d err=%v", got, store.logsLimit, err)
	}
	if len(store.enabled) != 1 || len(store.disabled) != 1 {
		t.Fatalf("enable/disable not delegated: %#v %#v", store.enabled, store.disabled)
	}
}

func TestManagerLoadJobsStoreError(t *testing.T) {
	wantErr := errors.New("store failed")
	manager, err := NewManager(ManagerOptions{
		Registry:  NewRegistry(),
		Store:     &fakeStore{listErr: wantErr},
		Scheduler: &fakeScheduler{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Reload(context.Background()); err != wantErr {
		t.Fatalf("error mismatch: got %v want %v", err, wantErr)
	}
}
