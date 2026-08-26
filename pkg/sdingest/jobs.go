package sdingest

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (c *Client) GetAppProfile(ctx context.Context) (AppProfile, error) {
	var result AppProfile
	err := c.do(ctx, http.MethodGet, "/v1/app/profile", nil, nil, "", &result)
	return result, err
}

func (c *Client) ListJobs(ctx context.Context, input ListJobsInput) (Page[Job], error) {
	query := paginationQuery(input.Page, input.Limit)
	setQuery(query, "provider", input.Provider)
	setQuery(query, "manifest_mode", input.ManifestMode)
	setQuery(query, "status", input.Status)
	setQuery(query, "phase", input.Phase)
	setQuery(query, "external_ref", input.ExternalRef)
	setQuery(query, "search", input.Search)
	var result Page[Job]
	err := c.do(ctx, http.MethodGet, "/v1/ingest/job/list", query, nil, "", &result)
	return result, err
}

func (c *Client) GetJob(ctx context.Context, jobID string) (Job, error) {
	query := url.Values{"job_id": {strings.TrimSpace(jobID)}}
	var result Job
	err := c.do(ctx, http.MethodGet, "/v1/ingest/job/detail", query, nil, "", &result)
	return result, err
}

func (c *Client) CreateJob(ctx context.Context, input CreateJobInput, idempotencyKey string) (Job, error) {
	key, err := requiredIdempotencyKey(idempotencyKey)
	if err != nil {
		return Job{}, err
	}
	var result Job
	err = c.do(ctx, http.MethodPost, "/v1/ingest/job/create", nil, input, key, &result)
	return result, err
}

func (c *Client) GetJobProgress(ctx context.Context, jobID string) (JobProgress, error) {
	query := url.Values{"job_id": {strings.TrimSpace(jobID)}}
	var result JobProgress
	err := c.do(ctx, http.MethodGet, "/v1/ingest/job/progress", query, nil, "", &result)
	return result, err
}

func (c *Client) GetItemProgress(ctx context.Context, jobID string, itemID string) (ItemProgress, error) {
	query := url.Values{
		"job_id":  {strings.TrimSpace(jobID)},
		"item_id": {strings.TrimSpace(itemID)},
	}
	var result ItemProgress
	err := c.do(ctx, http.MethodGet, "/v1/ingest/job/item-progress", query, nil, "", &result)
	return result, err
}

func (c *Client) GetJobManifest(ctx context.Context, jobID string, page int, limit int) (ManifestPage, error) {
	query := paginationQuery(page, limit)
	query.Set("job_id", strings.TrimSpace(jobID))
	var result ManifestPage
	err := c.do(ctx, http.MethodGet, "/v1/ingest/job/manifest", query, nil, "", &result)
	return result, err
}

func (c *Client) ConfirmJobManifest(ctx context.Context, jobID string, revision int64, hash string, idempotencyKey string) (Job, error) {
	return c.ConfirmJobManifestSelection(ctx, ConfirmJobManifestInput{
		JobID: jobID, Revision: revision, Hash: hash,
	}, idempotencyKey)
}

func (c *Client) ConfirmJobManifestSelection(ctx context.Context, input ConfirmJobManifestInput, idempotencyKey string) (Job, error) {
	key, err := requiredIdempotencyKey(idempotencyKey)
	if err != nil {
		return Job{}, err
	}
	input.JobID = strings.TrimSpace(input.JobID)
	input.Hash = strings.TrimSpace(input.Hash)
	for index := range input.ItemIDs {
		input.ItemIDs[index] = strings.TrimSpace(input.ItemIDs[index])
	}
	var result Job
	err = c.do(ctx, http.MethodPost, "/v1/ingest/job/manifest-confirm", nil, input, key, &result)
	return result, err
}

func (c *Client) CancelJob(ctx context.Context, jobID string, idempotencyKey string) (Job, error) {
	return c.jobAction(ctx, "/v1/ingest/job/cancel", jobID, idempotencyKey)
}

func (c *Client) RetryJob(ctx context.Context, jobID string, idempotencyKey string) (Job, error) {
	return c.jobAction(ctx, "/v1/ingest/job/retry", jobID, idempotencyKey)
}

func (c *Client) jobAction(ctx context.Context, path string, jobID string, idempotencyKey string) (Job, error) {
	key, err := requiredIdempotencyKey(idempotencyKey)
	if err != nil {
		return Job{}, err
	}
	input := struct {
		JobID string `json:"job_id"`
	}{JobID: strings.TrimSpace(jobID)}
	var result Job
	err = c.do(ctx, http.MethodPost, path, nil, input, key, &result)
	return result, err
}

func (c *Client) ListArtifacts(ctx context.Context, input ListArtifactsInput) (Page[Artifact], error) {
	query := paginationQuery(input.Page, input.Limit)
	setQuery(query, "job_id", input.JobID)
	setQuery(query, "status", input.Status)
	var result Page[Artifact]
	err := c.do(ctx, http.MethodGet, "/v1/ingest/job/artifact-list", query, nil, "", &result)
	return result, err
}

func (c *Client) GetArtifact(ctx context.Context, artifactID string) (Artifact, error) {
	query := url.Values{"artifact_id": {strings.TrimSpace(artifactID)}}
	var result Artifact
	err := c.do(ctx, http.MethodGet, "/v1/ingest/job/artifact-detail", query, nil, "", &result)
	return result, err
}

func (c *Client) GetArtifactAccess(ctx context.Context, artifactID string, ttl time.Duration) (ArtifactAccess, error) {
	query := url.Values{"artifact_id": {strings.TrimSpace(artifactID)}}
	if ttl > 0 {
		query.Set("ttl_seconds", strconv.FormatInt(int64(ttl/time.Second), 10))
	}
	var result ArtifactAccess
	err := c.do(ctx, http.MethodGet, "/v1/ingest/job/artifact-access", query, nil, "", &result)
	return result, err
}

func (c *Client) AcknowledgeArtifact(ctx context.Context, artifactID string, idempotencyKey string) (Artifact, error) {
	key, err := requiredIdempotencyKey(idempotencyKey)
	if err != nil {
		return Artifact{}, err
	}
	input := struct {
		ArtifactID string `json:"artifact_id"`
	}{ArtifactID: strings.TrimSpace(artifactID)}
	var result Artifact
	err = c.do(ctx, http.MethodPost, "/v1/ingest/job/artifact-ack", nil, input, key, &result)
	return result, err
}

func requiredIdempotencyKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrIdempotencyKeyRequired
	}
	return value, nil
}

func paginationQuery(page int, limit int) url.Values {
	query := make(url.Values)
	if page > 0 {
		query.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	return query
}

func setQuery(query url.Values, name string, value string) {
	if value = strings.TrimSpace(value); value != "" {
		query.Set(name, value)
	}
}
