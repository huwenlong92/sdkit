package crontab

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Manager struct {
	cfg       Config
	registry  *Registry
	store     Store
	repo      EntryRepository
	scheduler Scheduler
	runner    *Runner
	runtime   *RuntimeState
	logStore  LogStore
	streamer  LogStreamer
}

type ManagerOptions struct {
	Config     Config
	Registry   *Registry
	Store      Store
	Scheduler  Scheduler
	Locker     Locker
	Logger     LogWriter
	Repository EntryRepository
	Runner     *Runner
	Runtime    *RuntimeState
	LogStore   LogStore
	Streamer   LogStreamer
}

func NewManager(opts ManagerOptions) (*Manager, error) {
	if opts.Registry == nil {
		return nil, fmt.Errorf("crontab registry is required")
	}
	if opts.Scheduler == nil {
		return nil, fmt.Errorf("crontab scheduler is required")
	}
	if opts.Store == nil {
		opts.Store = NoopStore{}
	}

	if opts.Runtime == nil {
		opts.Runtime = NewRuntimeState()
	}
	runner := opts.Runner
	if runner == nil {
		runner = NewRunner(RunnerOptions{
			Config:   opts.Config,
			Registry: opts.Registry,
			Store:    opts.Store,
			Locker:   opts.Locker,
			Logger:   opts.Logger,
			LogStore: opts.LogStore,
			Streamer: opts.Streamer,
			Runtime:  opts.Runtime,
		})
	}
	if opts.Repository == nil {
		if repo, ok := opts.Store.(EntryRepository); ok {
			opts.Repository = repo
		}
	}
	if opts.LogStore == nil {
		if logStore, ok := opts.Store.(LogStore); ok {
			opts.LogStore = logStore
		}
	}
	if opts.Streamer == nil {
		opts.Streamer = NewMemoryLogStreamer()
	}

	return &Manager{
		cfg:       opts.Config,
		registry:  opts.Registry,
		store:     opts.Store,
		repo:      opts.Repository,
		scheduler: opts.Scheduler,
		runner:    runner,
		runtime:   opts.Runtime,
		logStore:  opts.LogStore,
		streamer:  opts.Streamer,
	}, nil
}

func (m *Manager) Start(ctx context.Context) error {
	jobs, err := m.loadJobs(ctx)
	if err != nil {
		return err
	}
	if err := m.scheduler.Reload(ctx, jobs); err != nil {
		return err
	}
	m.runtime.SetSchedule(jobsToEntries(jobs))
	return m.scheduler.Start(ctx)
}

func (m *Manager) Stop(ctx context.Context) error {
	return m.scheduler.Stop(ctx)
}

func (m *Manager) Shutdown(ctx context.Context) error {
	return m.Stop(ctx)
}

func (m *Manager) Reload(ctx context.Context) error {
	jobs, err := m.loadJobs(ctx)
	if err != nil {
		return err
	}
	if err := m.scheduler.Reload(ctx, jobs); err != nil {
		return err
	}
	m.runtime.SetSchedule(jobsToEntries(jobs))
	return nil
}

func (m *Manager) RunNow(ctx context.Context, name string) error {
	jobs, err := m.loadJobs(ctx)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if job.Name == name {
			go m.runner.Run(ctx, job)
			return nil
		}
	}
	return fmt.Errorf("crontab job not found: %s", name)
}

func (m *Manager) List(ctx context.Context) ([]JobStatus, error) {
	return m.store.ListJobs(ctx)
}

func (m *Manager) Status(ctx context.Context) ([]JobStatus, error) {
	return m.store.ListJobs(ctx)
}

func (m *Manager) Enable(ctx context.Context, name string) error {
	return m.store.Enable(ctx, name)
}

func (m *Manager) Disable(ctx context.Context, name string) error {
	return m.store.Disable(ctx, name)
}

func (m *Manager) Logs(ctx context.Context, name string, limit int) ([]RunLog, error) {
	return m.store.Logs(ctx, name, limit)
}

func (m *Manager) ListTemplates(ctx context.Context) ([]TemplateInfo, error) {
	return m.registry.ListTemplateInfo(), nil
}

func (m *Manager) ListDBTemplates(ctx context.Context) ([]TemplateInfo, error) {
	return m.registry.ListDBTemplateInfo(), nil
}

func (m *Manager) ListEntries(ctx context.Context) ([]EntryInfo, error) {
	if m.repo != nil {
		return m.repo.ListEntries(ctx)
	}
	statuses, err := m.store.ListJobs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]EntryInfo, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, EntryInfo{
			ID:          status.ID,
			Name:        status.Label,
			TemplateKey: status.Name,
			Spec:        status.Spec,
			Source:      status.Source,
			Enabled:     status.Enabled,
		})
	}
	return out, nil
}

func (m *Manager) GetEntry(ctx context.Context, id string) (EntryInfo, error) {
	if m.repo != nil {
		return m.repo.GetEntry(ctx, id)
	}
	entries, err := m.ListEntries(ctx)
	if err != nil {
		return EntryInfo{}, err
	}
	for _, entry := range entries {
		if entry.ID == id {
			return entry, nil
		}
	}
	return EntryInfo{}, fmt.Errorf("%w: %s", ErrEntryNotFound, id)
}

func (m *Manager) CreateEntry(ctx context.Context, req CreateEntryRequest) (EntryInfo, error) {
	if m.repo == nil {
		return EntryInfo{}, fmt.Errorf("crontab entry repository is required")
	}
	tpl, err := m.validateEntryRequest(req.TemplateKey, req.Spec, req.Payload, true)
	if err != nil {
		return EntryInfo{}, err
	}
	name := req.Name
	if name == "" {
		name = tpl.Name
	}
	return m.repo.Create(ctx, Entry{
		Name:        name,
		TemplateKey: req.TemplateKey,
		Spec:        req.Spec,
		Payload:     req.Payload,
		Source:      SourceDB,
		Enabled:     req.Enabled,
	})
}

func (m *Manager) UpdateEntry(ctx context.Context, id string, req UpdateEntryRequest) error {
	if m.repo == nil {
		return fmt.Errorf("crontab entry repository is required")
	}
	tpl, err := m.validateEntryRequest(req.TemplateKey, req.Spec, req.Payload, true)
	if err != nil {
		return err
	}
	name := req.Name
	if name == "" {
		name = tpl.Name
	}
	return m.repo.Update(ctx, Entry{
		ID:          id,
		Name:        name,
		TemplateKey: req.TemplateKey,
		Spec:        req.Spec,
		Payload:     req.Payload,
		Source:      SourceDB,
		Enabled:     req.Enabled,
	})
}

func (m *Manager) DeleteEntry(ctx context.Context, id string) error {
	if m.repo == nil {
		return fmt.Errorf("crontab entry repository is required")
	}
	return m.repo.Delete(ctx, id)
}

func (m *Manager) EnableEntry(ctx context.Context, id string) error {
	if m.repo != nil {
		return m.repo.UpdateEnabled(ctx, id, true)
	}
	return m.store.Enable(ctx, id)
}

func (m *Manager) DisableEntry(ctx context.Context, id string) error {
	if m.repo != nil {
		return m.repo.UpdateEnabled(ctx, id, false)
	}
	return m.store.Disable(ctx, id)
}

func (m *Manager) RunOnce(ctx context.Context, req RunOnceRequest) error {
	if req.EntryID != "" {
		return m.runEntryOnce(ctx, req)
	}
	if err := m.validateRunOnceRequest(req); err != nil {
		return err
	}
	entry := Entry{
		ID:          "manual." + uuid.NewString(),
		TemplateKey: req.TemplateKey,
		Name:        req.TemplateKey,
		Source:      SourceManual,
		Enabled:     true,
		Payload:     req.Payload,
		Timeout:     req.Timeout,
		Distributed: true,
		LockKey:     req.LockKey,
	}
	m.runner.Run(ctx, EntryToJob(entry))
	if m.runner.LastRunStatus(entry.ID) == StatusLocked {
		return ErrJobRunning
	}
	return nil
}

func (m *Manager) runEntryOnce(ctx context.Context, req RunOnceRequest) error {
	if m.repo == nil {
		return fmt.Errorf("crontab entry repository is required")
	}
	info, err := m.repo.GetEntry(ctx, req.EntryID)
	if err != nil {
		return err
	}
	entry := entryFromInfo(info)
	if req.Payload != "" {
		entry.Payload = req.Payload
	}
	if _, err := m.validateEntryRequest(entry.TemplateKey, entry.Spec, entry.Payload, false); err != nil {
		return err
	}
	tpl, _ := m.registry.Get(entry.TemplateKey)
	entry.Source = SourceManual
	entry.Enabled = true
	entry.Timeout = 0
	entry.Timeout = templateTimeout(tpl)
	if req.Timeout > 0 {
		entry.Timeout = req.Timeout
	}
	entry.Distributed = false
	entry.LockTTL = 0
	entry.LockKey = taskLockKey(entry.ID)
	if req.LockKey != "" {
		entry.LockKey = req.LockKey
	}
	m.runner.Run(ctx, EntryToJob(entry))
	if m.runner.LastRunStatus(entry.ID) == StatusLocked {
		return ErrJobRunning
	}
	return nil
}

func (m *Manager) ListRuntime(ctx context.Context) ([]RuntimeInfo, error) {
	return m.runtime.List(), nil
}

func (m *Manager) GetRuntime(ctx context.Context, id string) (RuntimeInfo, error) {
	info, ok := m.runtime.Get(id)
	if !ok {
		return RuntimeInfo{}, fmt.Errorf("crontab runtime not found: %s", id)
	}
	return info, nil
}

func (m *Manager) ListRuns(ctx context.Context, filter RunFilter) ([]RunLog, error) {
	if m.repo == nil {
		return nil, nil
	}
	return m.repo.ListRuns(ctx, filter)
}

func (m *Manager) ListRunLogs(ctx context.Context, filter RunLogFilter) ([]LogEvent, error) {
	if m.logStore == nil {
		return nil, nil
	}
	return m.logStore.ListRunLogs(ctx, filter)
}

func (m *Manager) StreamLogs(ctx context.Context, filter LogFilter) (<-chan LogEvent, error) {
	if m.streamer == nil {
		m.streamer = NewMemoryLogStreamer()
	}
	return m.streamer.Subscribe(ctx, filter)
}

func (m *Manager) Runner() *Runner {
	return m.runner
}

func (m *Manager) Config() Config {
	return m.cfg
}

func (m *Manager) loadJobs(ctx context.Context) ([]Job, error) {
	jobs := m.registry.BuiltinJobs()

	dbJobs, err := m.store.ListEnabledJobs(ctx)
	if err != nil {
		return nil, err
	}
	jobs = append(jobs, dbJobs...)

	loaded := make([]Job, 0, len(jobs))
	for i := range jobs {
		job, err := m.prepareJob(jobs[i])
		if err != nil {
			if isDBJob(jobs[i]) {
				m.markJobLoadFailed(ctx, normalizeDBJob(jobs[i]), validateStatus(err), err)
				continue
			}
			return nil, err
		}
		loaded = append(loaded, job)
	}

	return loaded, nil
}

func (m *Manager) prepareJob(job Job) (Job, error) {
	tpl, ok := m.registry.Get(job.Name)
	if !ok {
		return Job{}, ErrTemplateNotFound
	}
	if !tpl.Enabled {
		return Job{}, ErrTemplateDisabled
	}
	if job.Source == SourceDynamic {
		job.Source = SourceDB
	}
	if job.Source == SourceDB {
		job.ID = strings.TrimSpace(job.ID)
		if job.ID == "" {
			return Job{}, fmt.Errorf("crontab db job id is required")
		}
		if !strings.HasPrefix(job.ID, "db.") {
			job.ID = "db." + job.ID
		}
	}
	if job.Source == SourceDB && !tpl.AllowDB {
		return Job{}, ErrDynamicNotAllowed
	}
	if job.Spec != "" {
		if err := ValidateSpec(job.Spec); err != nil {
			return Job{}, err
		}
	}
	if job.Label == "" {
		job.Label = tpl.Name
	}
	if job.LockKey == "" {
		job.LockKey = taskLockKey(job.ID)
	}
	job.Mode = ModeLocal
	job.Queue = ""
	job.TaskType = ""
	job.Distributed = false
	job.LockTTL = 0
	job.Timeout = templateTimeout(tpl)
	return job, nil
}

func isDBJob(job Job) bool {
	return job.Source == SourceDB || job.Source == SourceDynamic
}

func normalizeDBJob(job Job) Job {
	if job.Source == SourceDynamic {
		job.Source = SourceDB
	}
	job.ID = strings.TrimSpace(job.ID)
	if job.Source == SourceDB && job.ID != "" && !strings.HasPrefix(job.ID, "db.") {
		job.ID = "db." + job.ID
	}
	return job
}

func (m *Manager) markJobLoadFailed(ctx context.Context, job Job, status Status, err error) {
	if status == "" {
		status = StatusFailed
	}
	startedAt := time.Now()
	runLog := RunLog{
		JobID:      job.ID,
		JobName:    job.Name,
		RunID:      uuid.NewString(),
		Source:     job.Source,
		Mode:       job.Mode,
		Spec:       job.Spec,
		Queue:      job.Queue,
		TaskType:   job.TaskType,
		Payload:    job.Payload,
		Status:     status,
		StartedAt:  startedAt.Unix(),
		FinishedAt: startedAt.Unix(),
		Host:       hostname(),
		InstanceID: m.cfg.InstanceID,
	}
	if err != nil {
		runLog.Error = err.Error()
	}
	_ = m.store.MarkFinished(ctx, runLog)
	if m.runtime != nil {
		m.runtime.MarkFinished(JobToEntry(job), runtimeStatus(status), startedAt, startedAt, err)
	}
}

func taskLockKey(entryID string) string {
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return ""
	}
	if isNumericID(entryID) {
		entryID = "db." + entryID
	}
	return "task:" + entryID
}

func isNumericID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func entryFromInfo(info EntryInfo) Entry {
	return Entry{
		ID:          info.ID,
		Name:        info.Name,
		TemplateKey: info.TemplateKey,
		Spec:        info.Spec,
		Payload:     info.Payload,
		Source:      info.Source,
		Enabled:     info.Enabled,
		Timeout:     info.Timeout,
		Distributed: info.Distributed,
		LockTTL:     info.LockTTL,
		LockKey:     info.LockKey,
	}
}

func (m *Manager) validateEntryRequest(templateKey, spec, payload string, requireDB bool) (Template, error) {
	templateKey = strings.TrimSpace(templateKey)
	if templateKey == "" {
		return Template{}, fmt.Errorf("crontab template key is required")
	}
	tpl, ok := m.registry.Get(templateKey)
	if !ok {
		return Template{}, ErrTemplateNotFound
	}
	if !tpl.Enabled {
		return Template{}, ErrTemplateDisabled
	}
	if requireDB && !tpl.AllowDB {
		return Template{}, ErrDynamicNotAllowed
	}
	if err := ValidateSpec(spec); err != nil {
		return Template{}, err
	}
	return tpl, nil
}

func (m *Manager) validateRunOnceRequest(req RunOnceRequest) error {
	templateKey := strings.TrimSpace(req.TemplateKey)
	if templateKey == "" {
		return fmt.Errorf("crontab template key is required")
	}
	tpl, ok := m.registry.Get(templateKey)
	if !ok {
		return ErrTemplateNotFound
	}
	if !tpl.Enabled {
		return ErrTemplateDisabled
	}
	return nil
}

func jobsToEntries(jobs []Job) []Entry {
	entries := make([]Entry, 0, len(jobs))
	for _, job := range jobs {
		entries = append(entries, JobToEntry(job))
	}
	return entries
}

var _ Service = (*Manager)(nil)
