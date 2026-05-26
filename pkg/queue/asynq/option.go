package asynq

import (
	"errors"
	"fmt"
	"strings"

	"github.com/huwenlong92/sdkit/core/queue"

	hibasynq "github.com/hibiken/asynq"
)

func validateOptions(opts queue.EnqueueOptions) error {
	if opts.Priority != 0 {
		return unsupported("asynq", queue.CapPriority)
	}
	if opts.RateLimitKey != "" {
		return unsupported("asynq", queue.CapRateLimit)
	}
	if opts.UniqueTTL > 0 && !capabilities()[queue.CapUnique] {
		return unsupported("asynq", queue.CapUnique)
	}
	return nil
}

func asynqOptions(opts queue.EnqueueOptions) []hibasynq.Option {
	out := make([]hibasynq.Option, 0, 9)
	if opts.Queue != "" {
		out = append(out, hibasynq.Queue(opts.Queue))
	}
	if opts.MaxRetry != nil {
		out = append(out, hibasynq.MaxRetry(*opts.MaxRetry))
	}
	if opts.Timeout > 0 {
		out = append(out, hibasynq.Timeout(opts.Timeout))
	}
	if !opts.Deadline.IsZero() {
		out = append(out, hibasynq.Deadline(opts.Deadline))
	}
	if !opts.ProcessAt.IsZero() {
		out = append(out, hibasynq.ProcessAt(opts.ProcessAt))
	}
	if opts.ProcessIn > 0 {
		out = append(out, hibasynq.ProcessIn(opts.ProcessIn))
	}
	if opts.TaskID != "" {
		out = append(out, hibasynq.TaskID(opts.TaskID))
	}
	if opts.UniqueTTL > 0 {
		out = append(out, hibasynq.Unique(opts.UniqueTTL))
	}
	if opts.Retention > 0 {
		out = append(out, hibasynq.Retention(opts.Retention))
	}
	if opts.Group != "" {
		out = append(out, hibasynq.Group(opts.Group))
	}
	return out
}

func listOptions(query queue.TaskQuery) []hibasynq.ListOption {
	limit := query.Limit
	if limit <= 0 {
		limit = 30
	}
	page := 1
	if query.Offset > 0 {
		page = query.Offset/limit + 1
	}
	return []hibasynq.ListOption{hibasynq.PageSize(limit), hibasynq.Page(page)}
}

func mapEnqueueError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, hibasynq.ErrDuplicateTask) ||
		errors.Is(err, hibasynq.ErrTaskIDConflict) ||
		strings.Contains(err.Error(), "task already exists") {
		return fmt.Errorf("%w: %v", queue.ErrTaskDuplicated, err)
	}
	return err
}
