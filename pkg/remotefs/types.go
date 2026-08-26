// Package remotefs defines provider-neutral contracts for reading hierarchical
// files from external file systems.
package remotefs

import (
	"context"
	"io"
	"time"
)

type EntryType string

const (
	EntryFile      EntryType = "file"
	EntryDirectory EntryType = "directory"
)

type Reference struct {
	ID   string `json:"id,omitempty"`
	Path string `json:"path"`
}

type Checksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type Entry struct {
	Reference Reference  `json:"reference"`
	Name      string     `json:"name"`
	Type      EntryType  `json:"type"`
	Size      int64      `json:"size,omitempty"`
	ModTime   *time.Time `json:"mod_time,omitempty"`
	Version   string     `json:"version,omitempty"`
	Checksum  *Checksum  `json:"checksum,omitempty"`
}

func (e Entry) IsDir() bool {
	return e.Type == EntryDirectory
}

type SortBy string

const (
	SortByName    SortBy = "name"
	SortBySize    SortBy = "size"
	SortByModTime SortBy = "mod_time"
)

type SortOrder string

const (
	SortAscending  SortOrder = "asc"
	SortDescending SortOrder = "desc"
)

type ListOptions struct {
	Cursor   string    `json:"cursor,omitempty"`
	PageSize int       `json:"page_size,omitempty"`
	SortBy   SortBy    `json:"sort_by,omitempty"`
	Order    SortOrder `json:"order,omitempty"`
}

type ListPage struct {
	Entries    []Entry `json:"entries"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

type OverwritePolicy string

const (
	OverwriteDeny    OverwritePolicy = "deny"
	OverwriteReplace OverwritePolicy = "replace"
)

type PartialFilePolicy string

const (
	PartialFileRemove PartialFilePolicy = "remove"
	PartialFileKeep   PartialFilePolicy = "keep"
)

type DownloadRequest struct {
	Reference       Reference         `json:"reference"`
	Destination     string            `json:"destination"`
	Overwrite       OverwritePolicy   `json:"overwrite,omitempty"`
	PartialFile     PartialFilePolicy `json:"partial_file,omitempty"`
	ExpectedVersion string            `json:"expected_version,omitempty"`
}

type DownloadResult struct {
	Path         string    `json:"path"`
	BytesWritten int64     `json:"bytes_written"`
	Checksum     *Checksum `json:"checksum,omitempty"`
}

type FileSystem interface {
	Driver() string
	Stat(ctx context.Context, ref Reference) (Entry, error)
	List(ctx context.Context, dir Reference, opts ListOptions) (ListPage, error)
	Close() error
}

type Downloader interface {
	Download(ctx context.Context, req DownloadRequest, sink ProgressSink) (DownloadResult, error)
}

type ShareRequest struct {
	URL      string `json:"url"`
	Password string `json:"password,omitempty"`
}

type StageRequest struct {
	Share       ShareRequest `json:"share"`
	Destination Reference    `json:"destination"`
}

type StageResult struct {
	Reference Reference `json:"reference"`
	Name      string    `json:"name,omitempty"`
	Duplicate bool      `json:"duplicate,omitempty"`
}

type ShareStager interface {
	StageShare(ctx context.Context, req StageRequest) (StageResult, error)
}

type OpenOptions struct {
	Offset int64 `json:"offset,omitempty"`
}

type Opener interface {
	Open(ctx context.Context, ref Reference, opts OpenOptions) (io.ReadCloser, error)
}

type RemoveRequest struct {
	Reference       Reference `json:"reference"`
	ExpectedVersion string    `json:"expected_version,omitempty"`
}

type Remover interface {
	Remove(ctx context.Context, req RemoveRequest) error
}

type HealthStatus string

const (
	HealthUnknown  HealthStatus = "unknown"
	HealthHealthy  HealthStatus = "healthy"
	HealthDegraded HealthStatus = "degraded"
)

type Health struct {
	Status  HealthStatus      `json:"status"`
	Version string            `json:"version,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

type HealthChecker interface {
	Check(ctx context.Context) (Health, error)
}

// Quota is a point-in-time capacity snapshot returned by a remote provider.
type Quota struct {
	TotalBytes int64 `json:"total_bytes"`
	UsedBytes  int64 `json:"used_bytes"`
}

// QuotaReader is an optional provider capability. Callers must discover it by
// type assertion instead of assuming every FileSystem supports capacity queries.
type QuotaReader interface {
	Quota(ctx context.Context) (Quota, error)
}
