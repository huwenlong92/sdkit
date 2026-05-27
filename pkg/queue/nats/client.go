package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/core/queue"

	natsgo "github.com/nats-io/nats.go"
)

type Queue struct {
	cfg           queue.Config
	conn          *natsgo.Conn
	js            natsgo.JetStreamContext
	stream        string
	subjectPrefix string
	durablePrefix string
	handlers      map[string]queue.HandlerFunc
	mws           []queue.Middleware
	subs          []*natsgo.Subscription
	done          chan struct{}
	closeOnce     sync.Once
	mu            sync.Mutex
	wg            sync.WaitGroup
}

type envelope struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	Queue          string            `json:"queue"`
	Payload        json.RawMessage   `json:"payload"`
	RetryCount     int               `json:"retry_count"`
	MaxRetry       int               `json:"max_retry"`
	MaxRetrySet    bool              `json:"max_retry_set,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
}

func New(cfg queue.Config) (*Queue, error) {
	cfg = cfg.Normalize()
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		addr = strings.TrimSpace(cfg.Redis.Addr)
	}
	if addr == "" {
		return nil, fmt.Errorf("queue nats addr is required")
	}
	conn, err := natsgo.Connect(addr)
	if err != nil {
		return nil, err
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, err
	}
	q := &Queue{
		cfg:           cfg,
		conn:          conn,
		js:            js,
		stream:        cfg.NATS.Stream,
		subjectPrefix: strings.TrimSuffix(cfg.NATS.SubjectPrefix, "."),
		durablePrefix: cfg.NATS.DurablePrefix,
		handlers:      map[string]queue.HandlerFunc{},
		done:          make(chan struct{}),
	}
	if err := q.ensureStream(); err != nil {
		conn.Close()
		return nil, err
	}
	return q, nil
}

func (q *Queue) Enqueue(ctx context.Context, task queue.Task, opts ...queue.Option) (*queue.TaskInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if q == nil || q.js == nil {
		return nil, queue.ErrNotInitialized
	}
	if task.Type == "" {
		return nil, fmt.Errorf("queue: task type is required")
	}
	payload, err := queue.MarshalPayload(task.Payload)
	if err != nil {
		return nil, err
	}
	applied := queue.ApplyOptions(opts)
	if task.Queue != "" {
		applied.Queue = task.Queue
	}
	if task.ID != "" {
		applied.TaskID = task.ID
	}
	if err := validateOptions(applied); err != nil {
		return nil, err
	}
	ctx, span := startEnqueueSpan(ctx, task.Type, applied)
	defer func() {
		recordQueueSpanError(span, err)
		span.End()
	}()
	queueName := firstNonEmpty(applied.Queue, queue.DefaultQueueName)
	maxRetry, maxRetrySet := q.maxRetryValue(applied.MaxRetry)
	headers := queue.CorrelationHeadersFromContext(ctx)
	for k, v := range task.Headers {
		if headers == nil {
			headers = map[string]string{}
		}
		headers[k] = v
	}
	env := envelope{
		ID:             applied.TaskID,
		Type:           task.Type,
		Queue:          queueName,
		Payload:        payload,
		Headers:        headers,
		MaxRetry:       maxRetry,
		MaxRetrySet:    maxRetrySet,
		TimeoutSeconds: int(applied.Timeout.Seconds()),
	}
	body, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	msg := &natsgo.Msg{
		Subject: q.subject(queueName, task.Type),
		Data:    body,
		Header:  natsgo.Header{},
	}
	for k, v := range headers {
		msg.Header.Set(k, v)
	}
	pubOpts := []natsgo.PubOpt{natsgo.Context(ctx), natsgo.ExpectStream(q.stream)}
	if env.ID != "" {
		pubOpts = append(pubOpts, natsgo.MsgId(env.ID))
	}
	ack, err := q.js.PublishMsg(msg, pubOpts...)
	if err != nil {
		return nil, err
	}
	if ack != nil && ack.Duplicate {
		return nil, queue.ErrTaskDuplicated
	}
	now := time.Now()
	setEnqueueEnvelopeAttributes(span, env)
	return &queue.TaskInfo{
		ID:        env.ID,
		Type:      env.Type,
		Queue:     env.Queue,
		State:     queue.StatePending,
		Payload:   payload,
		Headers:   headers,
		MaxRetry:  env.MaxRetry,
		Timeout:   applied.Timeout,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func validateOptions(opts queue.EnqueueOptions) error {
	if !opts.Deadline.IsZero() {
		return unsupported(queue.CapDeadline)
	}
	if opts.ProcessIn > 0 || !opts.ProcessAt.IsZero() {
		return unsupported(queue.CapDelay)
	}
	if opts.UniqueTTL > 0 {
		return unsupported(queue.CapUnique)
	}
	if opts.Priority != 0 {
		return unsupported(queue.CapPriority)
	}
	if opts.RateLimitKey != "" {
		return unsupported(queue.CapRateLimit)
	}
	return nil
}

func (q *Queue) BatchEnqueue(ctx context.Context, tasks []queue.Task, opts ...queue.Option) ([]*queue.TaskInfo, error) {
	out := make([]*queue.TaskInfo, 0, len(tasks))
	for _, task := range tasks {
		info, err := q.Enqueue(ctx, task, opts...)
		if err != nil {
			return out, err
		}
		out = append(out, info)
	}
	return out, nil
}

func (q *Queue) Handle(pattern string, handler queue.HandlerFunc) {
	if q == nil || handler == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	wrapped := handler
	for i := len(q.mws) - 1; i >= 0; i-- {
		wrapped = q.mws[i](wrapped)
	}
	q.handlers[pattern] = wrapped
}

func (q *Queue) Use(middlewares ...queue.Middleware) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.mws = append(q.mws, middlewares...)
}

func (q *Queue) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if q == nil || q.js == nil {
		return queue.ErrNotInitialized
	}
	cfg := q.cfg.Normalize()
	q.mu.Lock()
	for pattern, handler := range q.handlers {
		for queueName := range cfg.Queues {
			sub, err := q.js.PullSubscribe(q.subject(queueName, pattern), q.durable(queueName, pattern),
				natsgo.BindStream(q.stream),
				natsgo.ManualAck(),
				natsgo.AckExplicit(),
				natsgo.AckWait(cfg.NATS.AckWait),
				natsgo.MaxDeliver(q.maxDeliver()),
			)
			if err != nil {
				q.mu.Unlock()
				return err
			}
			q.subs = append(q.subs, sub)
			q.startPullLoop(ctx, sub, handler)
		}
	}
	q.mu.Unlock()
	<-ctx.Done()
	q.closeOnce.Do(func() { close(q.done) })
	q.wg.Wait()
	return ctx.Err()
}

func (q *Queue) Shutdown(context.Context) error {
	if q == nil {
		return nil
	}
	return q.Close()
}

func (q *Queue) Close() error {
	if q == nil || q.conn == nil {
		return nil
	}
	q.closeOnce.Do(func() { close(q.done) })
	q.mu.Lock()
	for _, sub := range q.subs {
		_ = sub.Unsubscribe()
	}
	q.subs = nil
	q.mu.Unlock()
	q.conn.Drain()
	q.conn.Close()
	q.wg.Wait()
	return nil
}

func (q *Queue) startPullLoop(ctx context.Context, sub *natsgo.Subscription, handler queue.HandlerFunc) {
	cfg := q.cfg.Normalize()
	batch := cfg.NATS.FetchBatch
	if batch <= 0 {
		batch = 1
	}
	fetchWait := cfg.NATS.FetchWait
	if fetchWait <= 0 {
		fetchWait = time.Second
	}
	sem := make(chan struct{}, maxInt(1, cfg.Concurrency))
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		for ctx.Err() == nil {
			select {
			case <-q.done:
				return
			default:
			}
			msgs, err := sub.Fetch(batch, natsgo.MaxWait(fetchWait))
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, natsgo.ErrTimeout) {
					continue
				}
				time.Sleep(200 * time.Millisecond)
				continue
			}
			for _, msg := range msgs {
				if msg == nil {
					continue
				}
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				case <-q.done:
					return
				}
				q.wg.Add(1)
				go func(m *natsgo.Msg) {
					defer q.wg.Done()
					defer func() { <-sem }()
					q.handleJetStreamMessage(m, handler)
				}(msg)
			}
		}
	}()
}

func (q *Queue) handleJetStreamMessage(msg *natsgo.Msg, handler queue.HandlerFunc) {
	var env envelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		_ = msg.Term()
		return
	}
	headers := map[string]string{}
	for k, values := range msg.Header {
		if len(values) > 0 {
			headers[k] = values[0]
		}
	}
	for k, v := range env.Headers {
		headers[k] = v
	}
	retryCount := env.RetryCount
	if meta, err := msg.Metadata(); err == nil && meta != nil && meta.NumDelivered > 0 {
		retryCount = int(meta.NumDelivered) - 1
	}
	ctx := queue.ContextFromCorrelationHeaders(context.Background(), headers)
	if env.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(env.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	queueMsg := &queue.Message{
		ID:         env.ID,
		Type:       env.Type,
		Payload:    []byte(env.Payload),
		Queue:      env.Queue,
		RetryCount: retryCount,
		MaxRetry:   env.MaxRetry,
		Headers:    headers,
	}
	ctx = queue.ContextWithMessage(ctx, queueMsg)
	ctx, span := startWorkerSpan(ctx, queueMsg)
	defer func() {
		if recovered := recover(); recovered != nil {
			recordQueueSpanPanic(span, recovered)
			span.End()
			panic(recovered)
		}
		span.End()
	}()
	if err := handler(ctx, queueMsg); err != nil {
		recordQueueSpanError(span, err)
		q.rejectMessage(msg, retryCount, env.MaxRetry, env.MaxRetrySet)
		return
	}
	_ = msg.Ack()
}

func (q *Queue) rejectMessage(msg *natsgo.Msg, retryCount int, taskMaxRetry int, maxRetrySet bool) {
	maxRetry := taskMaxRetry
	if !maxRetrySet {
		maxRetry = q.maxDeliver() - 1
	}
	if maxRetry <= 0 || retryCount >= maxRetry {
		_ = msg.Term()
		return
	}
	delay := q.cfg.Normalize().NATS.RetryDelay
	if delay <= 0 {
		delay = 5 * time.Second
	}
	_ = msg.NakWithDelay(delay)
}

func (q *Queue) ensureStream() error {
	if q == nil || q.js == nil {
		return queue.ErrNotInitialized
	}
	subj := q.subjectPrefix + ".>"
	info, err := q.js.StreamInfo(q.stream)
	if err == nil && info != nil {
		if subjectExists(info.Config.Subjects, subj) {
			return nil
		}
		cfg := info.Config
		cfg.Subjects = append(cfg.Subjects, subj)
		_, err = q.js.UpdateStream(&cfg)
		return err
	}
	if err != nil && !errors.Is(err, natsgo.ErrStreamNotFound) {
		return err
	}
	_, err = q.js.AddStream(&natsgo.StreamConfig{
		Name:       q.stream,
		Subjects:   []string{subj},
		Retention:  natsgo.LimitsPolicy,
		MaxAge:     q.cfg.NATS.MaxAge,
		Storage:    storageType(q.cfg.NATS.Storage),
		Replicas:   q.cfg.NATS.Replicas,
		Duplicates: q.cfg.NATS.Duplicates,
	})
	return err
}

func (q *Queue) Supports(cap queue.Capability) bool {
	return q.Capabilities()[cap]
}

func (q *Queue) Capabilities() map[queue.Capability]bool {
	return queue.CloneCapabilities(capabilities())
}

func (q *Queue) ListQueues(context.Context) ([]*queue.QueueInfo, error) {
	cfg := q.cfg.Normalize()
	out := make([]*queue.QueueInfo, 0, len(cfg.Queues))
	for name, priority := range cfg.Queues {
		out = append(out, &queue.QueueInfo{Name: name, State: queue.QueueRunning, Priority: priority, UpdatedAt: time.Now()})
	}
	return out, nil
}

func (q *Queue) GetQueue(_ context.Context, queueName string) (*queue.QueueInfo, error) {
	cfg := q.cfg.Normalize()
	priority := cfg.Queues[queueName]
	return &queue.QueueInfo{Name: queueName, State: queue.QueueRunning, Priority: priority, UpdatedAt: time.Now()}, nil
}

func (q *Queue) ListTasks(context.Context, queue.TaskQuery) ([]*queue.TaskInfo, error) {
	return nil, unsupported(queue.CapInspector)
}

func (q *Queue) GetTask(context.Context, string, string) (*queue.TaskInfo, error) {
	return nil, unsupported(queue.CapInspector)
}

func (q *Queue) DeleteTask(context.Context, string, string) error {
	return unsupported(queue.CapCancel)
}

func (q *Queue) RetryTask(context.Context, string, string) error {
	return unsupported(queue.CapInspector)
}

func (q *Queue) ArchiveTask(context.Context, string, string) error {
	return unsupported(queue.CapInspector)
}

func (q *Queue) CancelTask(context.Context, string, string) error {
	return unsupported(queue.CapCancel)
}

func (q *Queue) PauseQueue(context.Context, string) error {
	return unsupported(queue.CapPauseResume)
}

func (q *Queue) ResumeQueue(context.Context, string) error {
	return unsupported(queue.CapPauseResume)
}

func (q *Queue) subject(queueName string, taskType string) string {
	return q.subjectPrefix + "." + queueName + "." + taskType
}

func (q *Queue) durable(queueName string, pattern string) string {
	return sanitizeDurable(q.durablePrefix + "_" + queueName + "_" + pattern)
}

func (q *Queue) maxDeliver() int {
	maxDeliver := q.cfg.Normalize().NATS.MaxDeliver
	if maxDeliver <= 0 {
		return 1
	}
	return maxDeliver
}

func storageType(value string) natsgo.StorageType {
	if strings.EqualFold(strings.TrimSpace(value), "memory") {
		return natsgo.MemoryStorage
	}
	return natsgo.FileStorage
}

func subjectExists(subjects []string, target string) bool {
	for _, subject := range subjects {
		if subject == target {
			return true
		}
	}
	return false
}

func (q *Queue) maxRetryValue(value *int) (int, bool) {
	if value == nil {
		return q.maxDeliver() - 1, false
	}
	return *value, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if v := strings.TrimSpace(value); v != "" {
			return v
		}
	}
	return ""
}

var durableNameRE = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func sanitizeDurable(value string) string {
	value = strings.Trim(durableNameRE.ReplaceAllString(value, "_"), "_")
	if value == "" {
		return "sdkit_queue"
	}
	if len(value) > 200 {
		return value[:200]
	}
	return value
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ queue.QueueRunner = (*Queue)(nil)
