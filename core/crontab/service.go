package crontab

import "context"

type Service interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
	Reload(ctx context.Context) error

	ListTemplates(ctx context.Context) ([]TemplateInfo, error)
	ListDBTemplates(ctx context.Context) ([]TemplateInfo, error)

	ListEntries(ctx context.Context) ([]EntryInfo, error)
	GetEntry(ctx context.Context, id string) (EntryInfo, error)

	CreateEntry(ctx context.Context, req CreateEntryRequest) (EntryInfo, error)
	UpdateEntry(ctx context.Context, id string, req UpdateEntryRequest) error
	DeleteEntry(ctx context.Context, id string) error

	EnableEntry(ctx context.Context, id string) error
	DisableEntry(ctx context.Context, id string) error

	RunOnce(ctx context.Context, req RunOnceRequest) error

	ListRuntime(ctx context.Context) ([]RuntimeInfo, error)
	GetRuntime(ctx context.Context, id string) (RuntimeInfo, error)

	ListRuns(ctx context.Context, filter RunFilter) ([]RunLog, error)
	ListRunLogs(ctx context.Context, filter RunLogFilter) ([]LogEvent, error)
	StreamLogs(ctx context.Context, filter LogFilter) (<-chan LogEvent, error)
}
