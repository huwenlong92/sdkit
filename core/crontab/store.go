package crontab

import "context"

type Store interface {
	ListEnabledJobs(ctx context.Context) ([]Job, error)
	ListJobs(ctx context.Context) ([]JobStatus, error)
	MarkRunning(ctx context.Context, log RunLog) error
	MarkFinished(ctx context.Context, log RunLog) error
	Enable(ctx context.Context, name string) error
	Disable(ctx context.Context, name string) error
	Logs(ctx context.Context, name string, limit int) ([]RunLog, error)
}

type EntryRepository interface {
	ListEnabled(ctx context.Context) ([]Entry, error)
	ListEntries(ctx context.Context) ([]EntryInfo, error)
	GetEntry(ctx context.Context, id string) (EntryInfo, error)
	Create(ctx context.Context, entry Entry) (EntryInfo, error)
	Update(ctx context.Context, entry Entry) error
	Delete(ctx context.Context, id string) error
	UpdateEnabled(ctx context.Context, id string, enabled bool) error
	ListRuns(ctx context.Context, filter RunFilter) ([]RunLog, error)
}

type NoopStore struct{}

func (NoopStore) ListEnabledJobs(ctx context.Context) ([]Job, error)                 { return nil, nil }
func (NoopStore) ListJobs(ctx context.Context) ([]JobStatus, error)                  { return nil, nil }
func (NoopStore) MarkRunning(ctx context.Context, log RunLog) error                  { return nil }
func (NoopStore) MarkFinished(ctx context.Context, log RunLog) error                 { return nil }
func (NoopStore) Enable(ctx context.Context, name string) error                      { return nil }
func (NoopStore) Disable(ctx context.Context, name string) error                     { return nil }
func (NoopStore) Logs(ctx context.Context, name string, limit int) ([]RunLog, error) { return nil, nil }
