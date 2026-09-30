package sdingest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ArchiveJobStatusPending   = "pending"
	ArchiveJobStatusRunning   = "running"
	ArchiveJobStatusSucceeded = "succeeded"
	ArchiveJobStatusFailed    = "failed"

	ArchiveJobPhaseQueued      = "queued"
	ArchiveJobPhaseCompressing = "compressing"
	ArchiveJobPhaseUploading   = "uploading"
	ArchiveJobPhaseCompleted   = "completed"

	ArchiveCallbackEventSucceeded = "archive.succeeded"
	ArchiveCallbackEventFailed    = "archive.failed"
)

type ArchiveEntryInput struct {
	TargetID    string `json:"target_id"`
	SourcePath  string `json:"source_path"`
	ArchivePath string `json:"archive_path"`
	Size        int64  `json:"size,omitempty"`
}

type CreateArchiveJobInput struct {
	ExternalRef    string              `json:"external_ref,omitempty"`
	CallbackURL    string              `json:"callback_url,omitempty"`
	ArchiveName    string              `json:"archive_name"`
	RootDirectory  string              `json:"root_directory,omitempty"`
	OutputTargetID string              `json:"output_target_id"`
	Entries        []ArchiveEntryInput `json:"entries"`
}

type ListArchiveJobsInput struct {
	Page   int
	Limit  int
	Status string
	Search string
}

type ArchiveAppRef struct {
	AppID  string `json:"app_id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type ArchiveTargetRef struct {
	TargetID string `json:"target_id"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Bucket   string `json:"bucket"`
	Prefix   string `json:"prefix,omitempty"`
}

type ArchiveCallbackRef struct {
	URL string `json:"url"`
}

type ArchiveJob struct {
	JobID                 string              `json:"job_id"`
	AppID                 string              `json:"app_id"`
	CallbackURL           string              `json:"callback_url,omitempty"`
	ExternalRef           string              `json:"external_ref,omitempty"`
	ArchiveName           string              `json:"archive_name"`
	RootDirectory         string              `json:"root_directory,omitempty"`
	OutputTargetID        string              `json:"output_target_id"`
	ManifestHash          string              `json:"manifest_hash"`
	Status                string              `json:"status"`
	Phase                 string              `json:"phase"`
	Revision              int64               `json:"revision"`
	EntryTotal            int64               `json:"entry_total"`
	EntryDone             int64               `json:"entry_done"`
	TotalBytes            int64               `json:"total_bytes"`
	ProcessedBytes        int64               `json:"processed_bytes"`
	OutputKey             string              `json:"output_key,omitempty"`
	OutputSize            int64               `json:"output_size"`
	UploadedBytes         int64               `json:"uploaded_bytes"`
	OutputChecksum        string              `json:"output_checksum,omitempty"`
	HeartbeatAt           *time.Time          `json:"heartbeat_at,omitempty"`
	LastProgressAt        *time.Time          `json:"last_progress_at,omitempty"`
	ErrorCode             string              `json:"err_code,omitempty"`
	ErrorMessage          string              `json:"err_msg,omitempty"`
	StartedAt             *time.Time          `json:"started_at,omitempty"`
	UploadStartedAt       *time.Time          `json:"upload_started_at,omitempty"`
	FinishedAt            *time.Time          `json:"finished_at,omitempty"`
	CreatedAt             time.Time           `json:"created_at"`
	UpdatedAt             time.Time           `json:"updated_at"`
	ProgressPercent       int                 `json:"progress_percent"`
	UploadProgressPercent int                 `json:"upload_progress_percent"`
	ProcessBytesPerSecond int64               `json:"process_bytes_per_second"`
	UploadBytesPerSecond  int64               `json:"upload_bytes_per_second"`
	ProgressSource        string              `json:"progress_source"`
	CurrentEntryID        string              `json:"current_entry_id,omitempty"`
	App                   ArchiveAppRef       `json:"app"`
	OutputTarget          ArchiveTargetRef    `json:"output_target"`
	Callback              *ArchiveCallbackRef `json:"callback,omitempty"`
}

func (j ArchiveJob) Terminal() bool {
	return j.Status == ArchiveJobStatusSucceeded || j.Status == ArchiveJobStatusFailed
}

type ArchiveEntry struct {
	EntryID         string     `json:"entry_id"`
	Sequence        int32      `json:"sequence"`
	TargetID        string     `json:"target_id"`
	SourcePath      string     `json:"source_path"`
	ArchivePath     string     `json:"archive_path"`
	Size            int64      `json:"size"`
	Status          string     `json:"status"`
	ProcessedBytes  int64      `json:"processed_bytes"`
	ProgressPercent int        `json:"progress_percent"`
	HeartbeatAt     *time.Time `json:"heartbeat_at,omitempty"`
	LastProgressAt  *time.Time `json:"last_progress_at,omitempty"`
	ErrorCode       string     `json:"err_code,omitempty"`
	ErrorMessage    string     `json:"err_msg,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type ArchiveManifestPage struct {
	Job     ArchiveJob     `json:"job"`
	Entries []ArchiveEntry `json:"entries"`
	Total   int64          `json:"total"`
}

type ArchiveAccess struct {
	JobID     string    `json:"job_id"`
	TargetID  string    `json:"target_id"`
	Filename  string    `json:"filename"`
	Path      string    `json:"path"`
	ObjectURI string    `json:"object_uri"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ArchiveCallbackLog struct {
	EventID     string              `json:"event_id"`
	Event       string              `json:"event"`
	CallbackURL string              `json:"callback_url"`
	JobID       string              `json:"job_id,omitempty"`
	AttemptNo   int                 `json:"attempt_no"`
	Status      string              `json:"status"`
	LatencyMS   int64               `json:"latency_ms"`
	NextAt      *time.Time          `json:"next_at,omitempty"`
	Error       string              `json:"err,omitempty"`
	Response    string              `json:"response,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	Callback    *ArchiveCallbackRef `json:"callback,omitempty"`
}

type ArchiveCallbackLogDetail struct {
	ArchiveCallbackLog
	Payload json.RawMessage `json:"payload"`
}

type ArchiveCallbackEnvelope struct {
	EventID   string                      `json:"event_id"`
	Event     string                      `json:"event"`
	Timestamp time.Time                   `json:"timestamp"`
	Data      ArchiveCallbackEnvelopeData `json:"data"`
}

type ArchiveCallbackEnvelopeData struct {
	ArchiveJobID    string     `json:"archive_job_id"`
	TargetID        string     `json:"target_id"`
	ExternalRef     string     `json:"external_ref,omitempty"`
	Status          string     `json:"status"`
	Phase           string     `json:"phase"`
	Revision        int64      `json:"revision"`
	ErrorCode       string     `json:"error_code,omitempty"`
	ArchiveName     string     `json:"archive_name"`
	EntryTotal      int64      `json:"entry_total"`
	EntryDone       int64      `json:"entry_done"`
	TotalBytes      int64      `json:"total_bytes"`
	ProcessedBytes  int64      `json:"processed_bytes"`
	OutputKey       string     `json:"output_key,omitempty"`
	OutputSize      int64      `json:"output_size"`
	UploadedBytes   int64      `json:"uploaded_bytes"`
	OutputChecksum  string     `json:"output_checksum,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	UploadStartedAt *time.Time `json:"upload_started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	DurationMS      int64      `json:"duration_ms"`
}

type ArchiveJobEvent struct {
	Event string
	ID    string
	Job   ArchiveJob
}

func (c *Client) CreateArchiveJob(ctx context.Context, input CreateArchiveJobInput, idempotencyKey string) (ArchiveJob, error) {
	key, err := requiredIdempotencyKey(idempotencyKey)
	if err != nil {
		return ArchiveJob{}, err
	}
	var result ArchiveJob
	err = c.do(ctx, http.MethodPost, "/v1/archive/job/create", nil, input, key, &result)
	return result, err
}

func (c *Client) ListArchiveJobs(ctx context.Context, input ListArchiveJobsInput) (Page[ArchiveJob], error) {
	query := paginationQuery(input.Page, input.Limit)
	setQuery(query, "status", input.Status)
	setQuery(query, "search", input.Search)
	var result Page[ArchiveJob]
	err := c.do(ctx, http.MethodGet, "/v1/archive/job/list", query, nil, "", &result)
	return result, err
}

func (c *Client) GetArchiveJob(ctx context.Context, jobID string) (ArchiveJob, error) {
	query := url.Values{"job_id": {strings.TrimSpace(jobID)}}
	var result ArchiveJob
	err := c.do(ctx, http.MethodGet, "/v1/archive/job/detail", query, nil, "", &result)
	return result, err
}

func (c *Client) GetArchiveManifest(ctx context.Context, jobID string, page int, limit int) (ArchiveManifestPage, error) {
	query := paginationQuery(page, limit)
	query.Set("job_id", strings.TrimSpace(jobID))
	var result ArchiveManifestPage
	err := c.do(ctx, http.MethodGet, "/v1/archive/job/manifest", query, nil, "", &result)
	return result, err
}

func (c *Client) ListArchiveCallbackLogs(ctx context.Context, jobID string, page int, limit int) (Page[ArchiveCallbackLog], error) {
	query := paginationQuery(page, limit)
	query.Set("job_id", strings.TrimSpace(jobID))
	var result Page[ArchiveCallbackLog]
	err := c.do(ctx, http.MethodGet, "/v1/archive/job/callback-logs", query, nil, "", &result)
	return result, err
}

func (c *Client) GetArchiveCallbackLog(ctx context.Context, jobID string, eventID string, attemptNo int) (ArchiveCallbackLogDetail, error) {
	query := url.Values{
		"job_id":     {strings.TrimSpace(jobID)},
		"event_id":   {strings.TrimSpace(eventID)},
		"attempt_no": {strconv.Itoa(attemptNo)},
	}
	var result ArchiveCallbackLogDetail
	err := c.do(ctx, http.MethodGet, "/v1/archive/job/callback-log-detail", query, nil, "", &result)
	return result, err
}

func (c *Client) GetArchiveAccess(ctx context.Context, jobID string, ttl time.Duration) (ArchiveAccess, error) {
	query := url.Values{"job_id": {strings.TrimSpace(jobID)}}
	if ttl > 0 {
		query.Set("ttl_seconds", strconv.FormatInt(int64(ttl/time.Second), 10))
	}
	var result ArchiveAccess
	err := c.do(ctx, http.MethodGet, "/v1/archive/job/access", query, nil, "", &result)
	return result, err
}

func (c *Client) RetryArchiveJob(ctx context.Context, jobID string, idempotencyKey string) (ArchiveJob, error) {
	key, err := requiredIdempotencyKey(idempotencyKey)
	if err != nil {
		return ArchiveJob{}, err
	}
	input := struct {
		JobID string `json:"job_id"`
	}{JobID: strings.TrimSpace(jobID)}
	var result ArchiveJob
	err = c.do(ctx, http.MethodPost, "/v1/archive/job/retry", nil, input, key, &result)
	return result, err
}

// StreamArchiveJobEvents reads the authenticated SSE stream until the archive
// job emits a complete event, the handler returns an error, or ctx is canceled.
func (c *Client) StreamArchiveJobEvents(ctx context.Context, jobID string, handler func(ArchiveJobEvent) error) error {
	if ctx == nil {
		return ErrNilContext
	}
	if c == nil || c.httpClient == nil || c.baseURL == "" {
		return ErrInvalidConfig
	}
	if handler == nil {
		return fmt.Errorf("%w: archive event handler is required", ErrInvalidConfig)
	}
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.Authenticate(ctx)
		if err != nil {
			return err
		}
		response, err := c.openArchiveJobEvents(ctx, jobID, token.AccessToken)
		if err != nil {
			return err
		}
		if response.StatusCode == http.StatusUnauthorized && attempt == 0 {
			_ = response.Body.Close()
			c.invalidateToken(token.AccessToken)
			continue
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			streamErr := c.archiveStreamError(response)
			_ = response.Body.Close()
			return streamErr
		}
		err = c.consumeArchiveJobEvents(ctx, response.Body, handler)
		closeErr := response.Body.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	return ErrInvalidConfig
}

func (c *Client) openArchiveJobEvents(ctx context.Context, jobID string, accessToken string) (*http.Response, error) {
	endpoint, err := url.Parse(c.baseURL + "/v1/archive/job/events")
	if err != nil {
		return nil, errors.Join(ErrInvalidConfig, err)
	}
	query := endpoint.Query()
	query.Set("job_id", strings.TrimSpace(jobID))
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("User-Agent", c.userAgent)
	return c.httpClient.Do(request)
}

func (c *Client) archiveStreamError(response *http.Response) error {
	limit := c.maxResponseBytes
	if limit <= 0 {
		limit = defaultMaxResponseBytes
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > limit {
		return &ProtocolError{StatusCode: response.StatusCode, RequestID: requestID(response.Header), Err: errors.New("response body exceeds configured limit")}
	}
	var result envelope
	if err := json.Unmarshal(raw, &result); err != nil {
		return &ProtocolError{StatusCode: response.StatusCode, RequestID: requestID(response.Header), Err: err}
	}
	return &APIError{
		StatusCode: response.StatusCode, Code: result.ErrCode, SubCode: result.SubCode, Message: result.Message,
		Data: cloneRaw(result.Data), RequestID: requestID(response.Header),
		RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), c.clock()),
	}
}

func (c *Client) consumeArchiveJobEvents(ctx context.Context, reader io.Reader, handler func(ArchiveJobEvent) error) error {
	maxToken := int(c.maxResponseBytes)
	if maxToken < 64<<10 || int64(maxToken) != c.maxResponseBytes {
		maxToken = int(defaultMaxResponseBytes)
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), maxToken)
	event := ArchiveJobEvent{Event: "message"}
	data := make([]string, 0, 1)
	dispatch := func() (bool, error) {
		if len(data) == 0 {
			event = ArchiveJobEvent{Event: "message"}
			return false, nil
		}
		if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &event.Job); err != nil {
			return false, &ProtocolError{Err: err}
		}
		if err := handler(event); err != nil {
			return false, err
		}
		complete := event.Event == "complete"
		event = ArchiveJobEvent{Event: "message"}
		data = data[:0]
		return complete, nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			complete, err := dispatch()
			if err != nil || complete {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			value = ""
		} else {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "event":
			event.Event = value
		case "id":
			event.ID = value
		case "data":
			data = append(data, value)
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if len(data) > 0 {
		complete, err := dispatch()
		if err != nil || complete {
			return err
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return io.ErrUnexpectedEOF
}
