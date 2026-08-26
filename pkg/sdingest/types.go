// Package sdingest provides a typed client for the versioned SDIngest OpenAPI.
package sdingest

import (
	"encoding/json"
	"time"
)

const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusWaiting   = "waiting"
	JobStatusPartial   = "partial"
	JobStatusSucceeded = "succeeded"
	JobStatusFailed    = "failed"
	JobStatusCanceled  = "canceled"
)

const (
	ManifestModeAuto   = "auto"
	ManifestModeManual = "manual"
)

type Token struct {
	AccessToken string   `json:"access_token"`
	TokenType   string   `json:"token_type"`
	ExpiresIn   int64    `json:"expires_in"`
	Scope       []string `json:"scope"`

	expiresAt time.Time
}

type AppProfile struct {
	AppID             string          `json:"app_id"`
	Name              string          `json:"name"`
	Status            string          `json:"status"`
	Scope             []string        `json:"scope"`
	RateLimit         int64           `json:"rate_limit"`
	JobLimit          int             `json:"job_limit"`
	ByteLimit         int64           `json:"byte_limit"`
	StorageMode       string          `json:"storage_mode"`
	DefaultTarget     json.RawMessage `json:"default_target,omitempty"`
	DefaultCallbackID string          `json:"default_callback_id,omitempty"`
	DefaultFilterID   string          `json:"default_filter_id,omitempty"`
	ExpiresAt         *time.Time      `json:"expires_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type Source struct {
	Mode     string `json:"mode"`
	Provider string `json:"provider,omitempty"`
	URL      string `json:"url,omitempty"`
	Password string `json:"password,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Path     string `json:"path,omitempty"`
}

type FileFilter struct {
	IncludeExtensions []string `json:"include_extensions,omitempty"`
	ExcludeExtensions []string `json:"exclude_extensions,omitempty"`
	IncludeGlobs      []string `json:"include_globs,omitempty"`
	ExcludeGlobs      []string `json:"exclude_globs,omitempty"`
	MinSize           *int64   `json:"min_size,omitempty"`
	MaxSize           *int64   `json:"max_size,omitempty"`
	MaxFiles          *int64   `json:"max_files,omitempty"`
	MaxTotalBytes     *int64   `json:"max_total_bytes,omitempty"`
	MaxDepth          *int     `json:"max_depth,omitempty"`
}

type JobAppRef struct {
	AppID string `json:"app_id"`
	Name  string `json:"name"`
}

type JobTargetRef struct {
	TargetID string `json:"target_id"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Bucket   string `json:"bucket"`
	Prefix   string `json:"prefix,omitempty"`
}

type JobPoolRef struct {
	PoolID   string `json:"pool_id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

type Job struct {
	JobID          string          `json:"job_id"`
	SourceName     string          `json:"source_name,omitempty"`
	TargetID       string          `json:"target_id"`
	PoolID         string          `json:"pool_id,omitempty"`
	CallbackID     string          `json:"callback_id,omitempty"`
	ExternalRef    string          `json:"external_ref,omitempty"`
	Provider       string          `json:"provider"`
	SourceMode     string          `json:"source_mode"`
	ManifestMode   string          `json:"manifest_mode"`
	Filter         FileFilter      `json:"filter"`
	FailPolicy     string          `json:"fail_policy"`
	Priority       int16           `json:"priority"`
	Status         string          `json:"status"`
	Phase          string          `json:"phase"`
	WaitingReason  string          `json:"waiting_reason,omitempty"`
	Revision       int64           `json:"revision"`
	ItemTotal      int64           `json:"item_total"`
	ItemDone       int64           `json:"item_done"`
	ItemFailed     int64           `json:"item_failed"`
	TotalBytes     int64           `json:"total_bytes"`
	DownBytes      int64           `json:"down_bytes"`
	UpBytes        int64           `json:"up_bytes"`
	HeartbeatAt    *time.Time      `json:"heartbeat_at,omitempty"`
	LastProgressAt *time.Time      `json:"last_progress_at,omitempty"`
	NextRetryAt    *time.Time      `json:"next_retry_at,omitempty"`
	CancelAt       *time.Time      `json:"cancel_at,omitempty"`
	ErrorCode      string          `json:"err_code,omitempty"`
	ErrorMessage   string          `json:"err_msg,omitempty"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	App            JobAppRef       `json:"app"`
	Target         JobTargetRef    `json:"target"`
	Pool           *JobPoolRef     `json:"pool,omitempty"`
	Source         *Source         `json:"source,omitempty"`
	Raw            json.RawMessage `json:"-"`
}

func (j Job) Terminal() bool {
	switch j.Status {
	case JobStatusPartial, JobStatusSucceeded, JobStatusFailed, JobStatusCanceled:
		return true
	default:
		return false
	}
}

type ListJobsInput struct {
	Page         int
	Limit        int
	Provider     string
	ManifestMode string
	Status       string
	Phase        string
	ExternalRef  string
	Search       string
}

type CreateJobInput struct {
	ExternalRef  string     `json:"external_ref,omitempty"`
	Source       Source     `json:"source"`
	Filter       FileFilter `json:"filter,omitempty"`
	ManifestMode string     `json:"manifest_mode,omitempty"`
	TargetID     string     `json:"target_id,omitempty"`
	PoolID       string     `json:"pool_id,omitempty"`
	CallbackID   string     `json:"callback_id,omitempty"`
	FailPolicy   string     `json:"fail_policy,omitempty"`
	Priority     int16      `json:"priority,omitempty"`
}

type ConfirmJobManifestInput struct {
	JobID    string   `json:"job_id"`
	Revision int64    `json:"revision"`
	Hash     string   `json:"hash"`
	ItemIDs  []string `json:"item_ids,omitempty"`
}

type Page[T any] struct {
	List  []T   `json:"list"`
	Total int64 `json:"total"`
}

type TransferProgress struct {
	TransferredBytes int64      `json:"transferred_bytes"`
	TotalBytes       int64      `json:"total_bytes"`
	Percent          *float64   `json:"percent"`
	BytesPerSecond   int64      `json:"bytes_per_second"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	LastProgressAt   *time.Time `json:"last_progress_at,omitempty"`
}

type Liveness struct {
	State             string     `json:"state"`
	IsAlive           bool       `json:"is_alive"`
	AttemptID         string     `json:"attempt_id,omitempty"`
	WorkerID          string     `json:"worker_id,omitempty"`
	NodeID            string     `json:"node_id,omitempty"`
	HeartbeatAt       *time.Time `json:"heartbeat_at,omitempty"`
	StateChangedAt    *time.Time `json:"state_changed_at,omitempty"`
	LastProgressAt    *time.Time `json:"last_progress_at,omitempty"`
	NoProgressSeconds int64      `json:"no_progress_seconds"`
	SnapshotRevision  int64      `json:"snapshot_revision"`
}

type JobProgress struct {
	JobID            string           `json:"job_id"`
	CurrentItemID    string           `json:"current_item_id,omitempty"`
	CurrentAttemptID string           `json:"current_attempt_id,omitempty"`
	Source           string           `json:"source"`
	Status           string           `json:"status"`
	Phase            string           `json:"phase"`
	ItemTotal        int64            `json:"item_total"`
	ItemDone         int64            `json:"item_done"`
	ItemFailed       int64            `json:"item_failed"`
	Download         TransferProgress `json:"download"`
	Upload           TransferProgress `json:"upload"`
	Overall          TransferProgress `json:"overall"`
	Liveness         Liveness         `json:"liveness"`
}

func (p JobProgress) Terminal() bool {
	return Job{Status: p.Status}.Terminal()
}

type ItemProgress struct {
	JobID    string           `json:"job_id"`
	ItemID   string           `json:"item_id"`
	Source   string           `json:"source"`
	Status   string           `json:"status"`
	Phase    string           `json:"phase"`
	Download TransferProgress `json:"download"`
	Upload   TransferProgress `json:"upload"`
	Overall  TransferProgress `json:"overall"`
	Liveness Liveness         `json:"liveness"`
}

type Manifest struct {
	JobID       string          `json:"job_id"`
	Revision    int64           `json:"revision"`
	Hash        string          `json:"hash"`
	Filter      json.RawMessage `json:"filter"`
	Status      string          `json:"status"`
	ItemCount   int64           `json:"item_count"`
	TotalBytes  int64           `json:"total_bytes"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
	ConfirmedAt *time.Time      `json:"confirmed_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

type ManifestItem struct {
	ItemID     string          `json:"item_id"`
	ArtifactID string          `json:"artifact_id,omitempty"`
	Path       string          `json:"path"`
	Name       string          `json:"name"`
	Size       int64           `json:"size"`
	MTime      *time.Time      `json:"mtime,omitempty"`
	Checksum   string          `json:"checksum,omitempty"`
	Selected   bool            `json:"selected"`
	SkipReason string          `json:"skip_reason,omitempty"`
	Status     string          `json:"status"`
	Phase      string          `json:"phase"`
	LocalSize  int64           `json:"local_size"`
	MIME       string          `json:"mime,omitempty"`
	HashType   string          `json:"hash_type,omitempty"`
	HashValue  string          `json:"hash_value,omitempty"`
	Media      json.RawMessage `json:"media,omitempty"`
	DownBytes  int64           `json:"down_bytes"`
	UpBytes    int64           `json:"up_bytes"`
	ErrorCode  string          `json:"err_code,omitempty"`
	ErrorMsg   string          `json:"err_msg,omitempty"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

type ManifestPage struct {
	Manifest Manifest       `json:"manifest"`
	Items    []ManifestItem `json:"items"`
	Total    int64          `json:"total"`
}

type ArtifactJobRef struct {
	JobID       string     `json:"job_id"`
	SourceName  string     `json:"source_name,omitempty"`
	ExternalRef string     `json:"external_ref,omitempty"`
	Provider    string     `json:"provider"`
	Status      string     `json:"status"`
	Phase       string     `json:"phase"`
	CreatedAt   time.Time  `json:"created_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

type Artifact struct {
	ArtifactID     string         `json:"artifact_id"`
	JobID          string         `json:"job_id"`
	ItemID         string         `json:"item_id"`
	TargetID       string         `json:"target_id"`
	TargetName     string         `json:"target_name"`
	TargetDriver   string         `json:"target_driver"`
	TargetEndpoint string         `json:"target_endpoint"`
	TargetBucket   string         `json:"target_bucket"`
	ObjectURI      string         `json:"object_uri"`
	Key            string         `json:"key"`
	Name           string         `json:"name"`
	Path           string         `json:"path"`
	MIME           string         `json:"mime,omitempty"`
	Size           int64          `json:"size"`
	Checksum       string         `json:"checksum,omitempty"`
	ETag           string         `json:"etag,omitempty"`
	Status         string         `json:"status"`
	AckAt          *time.Time     `json:"ack_at,omitempty"`
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	Job            ArtifactJobRef `json:"job"`
}

type ListArtifactsInput struct {
	Page   int
	Limit  int
	JobID  string
	Status string
}

type ArtifactAccess struct {
	ArtifactID string    `json:"artifact_id"`
	ObjectURI  string    `json:"object_uri"`
	URL        string    `json:"url"`
	ExpiresAt  time.Time `json:"expires_at"`
}
