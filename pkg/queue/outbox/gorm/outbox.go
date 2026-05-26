package gormoutbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/huwenlong92/sdkit/core/jsonx"
	"github.com/huwenlong92/sdkit/core/queue"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OutboxStatusPending = "pending"
	OutboxStatusSent    = "sent"
	OutboxStatusFailed  = "failed"
)

type OutboxRecord struct {
	ID          uint64         `json:"id" gorm:"primaryKey;autoIncrement;comment:主键"`
	TaskID      string         `json:"task_id" gorm:"column:task_id;size:191;index;comment:任务ID"`
	Queue       string         `json:"queue" gorm:"column:queue;size:64;not null;index:idx_queue_outbox_status_available,priority:2;comment:队列"`
	Type        string         `json:"type" gorm:"column:type;size:191;not null;index;comment:任务类型"`
	Payload     datatypes.JSON `json:"payload" gorm:"column:payload;type:jsonb;not null;comment:任务载荷"`
	Headers     datatypes.JSON `json:"headers" gorm:"column:headers;type:jsonb;not null;default:'{}'::jsonb;comment:任务头"`
	Options     datatypes.JSON `json:"options" gorm:"column:options;type:jsonb;not null;default:'{}'::jsonb;comment:投递选项"`
	Status      string         `json:"status" gorm:"column:status;size:32;not null;index:idx_queue_outbox_status_available,priority:1;comment:状态"`
	Attempts    int            `json:"attempts" gorm:"column:attempts;not null;default:0;comment:投递尝试次数"`
	LastError   string         `json:"last_error" gorm:"column:last_error;type:text;comment:最后错误"`
	AvailableAt time.Time      `json:"available_at" gorm:"column:available_at;not null;index:idx_queue_outbox_status_available,priority:3;comment:可投递时间"`
	SentAt      *time.Time     `json:"sent_at" gorm:"column:sent_at;comment:投递成功时间"`
	CreatedAt   time.Time      `json:"created_at,omitempty" gorm:"column:created_at;comment:创建时间"`
	UpdatedAt   time.Time      `json:"updated_at,omitempty" gorm:"column:updated_at;comment:更新时间"`
}

func (OutboxRecord) TableName() string {
	return "system_queue_outbox"
}

type outboxOptions struct {
	Queue            string     `json:"queue,omitempty"`
	TaskID           string     `json:"task_id,omitempty"`
	MaxRetry         *int       `json:"max_retry,omitempty"`
	Timeout          int64      `json:"timeout,omitempty"`
	Deadline         *time.Time `json:"deadline,omitempty"`
	ProcessAt        *time.Time `json:"process_at,omitempty"`
	ProcessIn        int64      `json:"process_in,omitempty"`
	UniqueTTL        int64      `json:"unique_ttl,omitempty"`
	Retention        int64      `json:"retention,omitempty"`
	Group            string     `json:"group,omitempty"`
	Priority         int        `json:"priority,omitempty"`
	RateLimitKey     string     `json:"rate_limit_key,omitempty"`
	Trace            *bool      `json:"trace,omitempty"`
	AutoRetryEnabled bool       `json:"auto_retry_enabled,omitempty"`
	AutoRetryMax     int        `json:"auto_retry_max,omitempty"`
	AutoRetryDelay   int64      `json:"auto_retry_delay,omitempty"`
}

type GormOutbox struct {
	db     *gorm.DB
	client queue.Client
}

func New(db *gorm.DB, client queue.Client) *GormOutbox {
	return NewGormOutbox(db, client)
}

func NewGormOutbox(db *gorm.DB, client queue.Client) *GormOutbox {
	return &GormOutbox{db: db, client: client}
}

func Migrate(ctx context.Context, db *gorm.DB) error {
	return MigrateOutbox(ctx, db)
}

func MigrateOutbox(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return queue.ErrNotInitialized
	}
	return db.WithContext(ctx).AutoMigrate(&OutboxRecord{})
}

func (o *GormOutbox) Save(ctx context.Context, task queue.Task, opts ...queue.Option) error {
	return o.SaveBatch(ctx, queue.NewOutboxTask(task, opts...))
}

func (o *GormOutbox) SaveBatch(ctx context.Context, tasks ...queue.OutboxTask) error {
	if o == nil || o.db == nil {
		return queue.ErrNotInitialized
	}
	if len(tasks) == 0 {
		return nil
	}
	records := make([]OutboxRecord, 0, len(tasks))
	now := time.Now()
	for _, item := range tasks {
		record, err := outboxRecordFromTask(ctx, item.Task, now, item.Options...)
		if err != nil {
			return err
		}
		records = append(records, record)
	}
	return o.db.WithContext(ctx).Create(&records).Error
}

func outboxRecordFromTask(ctx context.Context, task queue.Task, availableAt time.Time, opts ...queue.Option) (OutboxRecord, error) {
	if task.Type == "" {
		return OutboxRecord{}, fmt.Errorf("queue outbox task type is required")
	}
	payload, err := queue.MarshalPayload(task.Payload)
	if err != nil {
		return OutboxRecord{}, err
	}
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	applied := queue.ApplyOptions(opts)
	if task.Queue != "" {
		applied.Queue = task.Queue
	}
	if task.ID != "" {
		applied.TaskID = task.ID
	}
	headersMap := outboxHeadersFromContext(ctx)
	for k, v := range task.Headers {
		if headersMap == nil {
			headersMap = map[string]string{}
		}
		headersMap[k] = v
	}
	headers, err := jsonBytes(headersMap)
	if err != nil {
		return OutboxRecord{}, fmt.Errorf("%w: %v", queue.ErrInvalidPayload, err)
	}
	options, err := jsonBytes(toOutboxOptions(applied))
	if err != nil {
		return OutboxRecord{}, fmt.Errorf("%w: %v", queue.ErrInvalidPayload, err)
	}
	return OutboxRecord{
		TaskID:      applied.TaskID,
		Queue:       applied.Queue,
		Type:        task.Type,
		Payload:     datatypes.JSON(payload),
		Headers:     datatypes.JSON(headers),
		Options:     datatypes.JSON(options),
		Status:      OutboxStatusPending,
		AvailableAt: availableAt,
	}, nil
}

func (o *GormOutbox) Flush(ctx context.Context, limit int) error {
	if o == nil || o.db == nil || o.client == nil {
		return queue.ErrNotInitialized
	}
	if limit <= 0 {
		limit = 100
	}
	var firstErr error
	err := o.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []OutboxRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND available_at <= ?", []string{OutboxStatusPending, OutboxStatusFailed}, time.Now()).
			Order("id ASC").
			Limit(limit).
			Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := o.flushOne(ctx, tx, row); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return firstErr
}

func (o *GormOutbox) flushOne(ctx context.Context, tx *gorm.DB, row OutboxRecord) error {
	headers, err := decodeHeaders(row.Headers)
	if err != nil {
		return markOutboxFailed(tx, row, err)
	}
	opts, err := decodeOutboxOptions(row.Options)
	if err != nil {
		return markOutboxFailed(tx, row, err)
	}
	enqueueCtx := outboxEnqueueContext(ctx, headers)
	_, err = o.client.Enqueue(enqueueCtx, queue.Task{
		ID:      row.TaskID,
		Type:    row.Type,
		Queue:   row.Queue,
		Payload: []byte(row.Payload),
		Headers: headers,
	}, opts...)
	if err != nil {
		if errors.Is(err, queue.ErrTaskDuplicated) {
			return markOutboxSent(tx, row)
		}
		_ = markOutboxFailed(tx, row, err)
		return err
	}
	return markOutboxSent(tx, row)
}

func markOutboxSent(tx *gorm.DB, row OutboxRecord) error {
	now := time.Now()
	return tx.Model(&OutboxRecord{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"status":     OutboxStatusSent,
			"sent_at":    &now,
			"last_error": "",
		}).Error
}

func markOutboxFailed(tx *gorm.DB, row OutboxRecord, err error) error {
	attempts := row.Attempts + 1
	delay := time.Duration(attempts) * time.Second
	if delay > time.Minute {
		delay = time.Minute
	}
	return tx.Model(&OutboxRecord{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"status":       OutboxStatusFailed,
			"attempts":     attempts,
			"last_error":   err.Error(),
			"available_at": time.Now().Add(delay),
		}).Error
}

func toOutboxOptions(opts queue.EnqueueOptions) outboxOptions {
	out := outboxOptions{
		Queue:            opts.Queue,
		TaskID:           opts.TaskID,
		MaxRetry:         opts.MaxRetry,
		Timeout:          int64(opts.Timeout),
		ProcessIn:        int64(opts.ProcessIn),
		UniqueTTL:        int64(opts.UniqueTTL),
		Retention:        int64(opts.Retention),
		Group:            opts.Group,
		Priority:         opts.Priority,
		RateLimitKey:     opts.RateLimitKey,
		Trace:            &opts.Trace,
		AutoRetryEnabled: opts.AutoRetryEnabled,
		AutoRetryMax:     opts.AutoRetryMax,
		AutoRetryDelay:   int64(opts.AutoRetryDelay),
	}
	if !opts.Deadline.IsZero() {
		out.Deadline = &opts.Deadline
	}
	if !opts.ProcessAt.IsZero() {
		out.ProcessAt = &opts.ProcessAt
	}
	return out
}

func fromOutboxOptions(opts outboxOptions) []queue.Option {
	out := make([]queue.Option, 0, 14)
	if opts.Queue != "" {
		out = append(out, queue.Queue(opts.Queue))
	}
	if opts.TaskID != "" {
		out = append(out, queue.TaskID(opts.TaskID))
	}
	if opts.MaxRetry != nil {
		out = append(out, queue.MaxRetry(*opts.MaxRetry))
	}
	if opts.Timeout > 0 {
		out = append(out, queue.Timeout(time.Duration(opts.Timeout)))
	}
	if opts.Deadline != nil {
		out = append(out, queue.Deadline(*opts.Deadline))
	}
	if opts.ProcessAt != nil {
		out = append(out, queue.ProcessAt(*opts.ProcessAt))
	}
	if opts.ProcessIn > 0 {
		out = append(out, queue.ProcessIn(time.Duration(opts.ProcessIn)))
	}
	if opts.UniqueTTL > 0 {
		out = append(out, queue.Unique(time.Duration(opts.UniqueTTL)))
	}
	if opts.Retention > 0 {
		out = append(out, queue.Retention(time.Duration(opts.Retention)))
	}
	if opts.Group != "" {
		out = append(out, queue.Group(opts.Group))
	}
	if opts.Priority != 0 {
		out = append(out, queue.WithPriority(opts.Priority))
	}
	if opts.RateLimitKey != "" {
		out = append(out, queue.WithRateLimitKey(opts.RateLimitKey))
	}
	if opts.Trace != nil {
		out = append(out, queue.WithTrace(*opts.Trace))
	}
	if opts.AutoRetryEnabled {
		out = append(out, queue.AutoRetry(opts.AutoRetryMax, time.Duration(opts.AutoRetryDelay)))
	}
	return out
}

func jsonBytes(v any) ([]byte, error) {
	if v == nil {
		return []byte("{}"), nil
	}
	b, err := jsonx.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || string(b) == "null" {
		return []byte("{}"), nil
	}
	return b, nil
}

func decodeHeaders(raw datatypes.JSON) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var headers map[string]string
	if err := jsonx.Unmarshal(raw, &headers); err != nil {
		return nil, fmt.Errorf("%w: %v", queue.ErrInvalidPayload, err)
	}
	return headers, nil
}

func decodeOutboxOptions(raw datatypes.JSON) ([]queue.Option, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var opts outboxOptions
	if err := jsonx.Unmarshal(raw, &opts); err != nil {
		return nil, fmt.Errorf("%w: %v", queue.ErrInvalidPayload, err)
	}
	return fromOutboxOptions(opts), nil
}

func outboxHeadersFromContext(ctx context.Context) map[string]string {
	return queue.CorrelationHeadersFromContext(ctx)
}

func outboxEnqueueContext(ctx context.Context, headers map[string]string) context.Context {
	return queue.ContextFromCorrelationHeaders(ctx, headers)
}
