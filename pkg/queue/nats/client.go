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

	corequeue "github.com/huwenlong92/sdkit/core/queue"

	natsgo "github.com/nats-io/nats.go"
)

type Queue struct {
	cfg           corequeue.Config
	conn          *natsgo.Conn
	js            natsgo.JetStreamContext
	stream        string
	subjectPrefix string
	durablePrefix string
	handlers      map[string]corequeue.HandlerFunc
	mws           []corequeue.Middleware
	subs          []*natsgo.Subscription
	done          chan struct{}
	closeOnce     sync.Once
	mu            sync.Mutex
	wg            sync.WaitGroup
}

type envelope struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Queue      string            `json:"queue"`
	Payload    json.RawMessage   `json:"payload"`
	RetryCount int               `json:"retry_count"`
	MaxRetry   int               `json:"max_retry"`
	Headers    map[string]string `json:"headers,omitempty"`
}

func New(cfg corequeue.Config) (*Queue, error) {
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
		handlers:      map[string]corequeue.HandlerFunc{},
		done:          make(chan struct{}),
	}
	if err := q.ensureStream(); err != nil {
		conn.Close()
		return nil, err
	}
	return q, nil
}

func (q *Queue) Enqueue(ctx context.Context, task corequeue.Task, opts ...corequeue.Option) (*corequeue.TaskInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if q == nil || q.js == nil {
		return nil, corequeue.ErrNotInitialized
	}
	if task.Type == "" {
		return nil, fmt.Errorf("queue: task type is required")
	}
	payload, err := corequeue.MarshalPayload(task.Payload)
	if err != nil {
		return nil, err
	}
	applied := corequeue.ApplyOptions(opts)
	if task.Queue != "" {
		applied.Queue = task.Queue
	}
	if task.ID != "" {
		applied.TaskID = task.ID
	}
	if applied.UniqueTTL > 0 {
		return nil, unsupported(corequeue.CapUnique)
	}
	if applied.ProcessIn > 0 || !applied.ProcessAt.IsZero() {
		return nil, unsupported(corequeue.CapDelay)
	}
	queueName := firstNonEmpty(applied.Queue, corequeue.DefaultQueueName)
	headers := corequeue.CorrelationHeadersFromContext(ctx)
	for k, v := range task.Headers {
		if headers == nil {
			headers = map[string]string{}
		}
		headers[k] = v
	}
	env := envelope{
		ID:       applied.TaskID,
		Type:     task.Type,
		Queue:    queueName,
		Payload:  payload,
		Headers:  headers,
		MaxRetry: maxRetryValue(applied.MaxRetry),
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
		return nil, corequeue.ErrTaskDuplicated
	}
	now := time.Now()
	return &corequeue.TaskInfo{
		ID:        env.ID,
		Type:      env.Type,
		Queue:     env.Queue,
		State:     corequeue.StatePending,
		Payload:   payload,
		Headers:   headers,
		MaxRetry:  env.MaxRetry,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (q *Queue) BatchEnqueue(ctx context.Context, tasks []corequeue.Task, opts ...corequeue.Option) ([]*corequeue.TaskInfo, error) {
	out := make([]*corequeue.TaskInfo, 0, len(tasks))
	for _, task := range tasks {
		info, err := q.Enqueue(ctx, task, opts...)
		if err != nil {
			return out, err
		}
		out = append(out, info)
	}
	return out, nil
}

func (q *Queue) Handle(pattern string, handler corequeue.HandlerFunc) {
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

func (q *Queue) Use(middlewares ...corequeue.Middleware) {
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
		return corequeue.ErrNotInitialized
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

func (q *Queue) startPullLoop(ctx context.Context, sub *natsgo.Subscription, handler corequeue.HandlerFunc) {
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

func (q *Queue) handleJetStreamMessage(msg *natsgo.Msg, handler corequeue.HandlerFunc) {
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
	ctx := corequeue.ContextFromCorrelationHeaders(context.Background(), headers)
	queueMsg := &corequeue.Message{
		ID:         env.ID,
		Type:       env.Type,
		Payload:    []byte(env.Payload),
		Queue:      env.Queue,
		RetryCount: retryCount,
		MaxRetry:   env.MaxRetry,
		Headers:    headers,
	}
	ctx = corequeue.ContextWithMessage(ctx, queueMsg)
	if err := handler(ctx, queueMsg); err != nil {
		q.rejectMessage(msg, retryCount, env.MaxRetry)
		return
	}
	_ = msg.Ack()
}

func (q *Queue) rejectMessage(msg *natsgo.Msg, retryCount int, taskMaxRetry int) {
	maxRetry := taskMaxRetry
	if maxRetry <= 0 {
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
		return corequeue.ErrNotInitialized
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

func (q *Queue) Supports(cap corequeue.Capability) bool {
	return q.Capabilities()[cap]
}

func (q *Queue) Capabilities() map[corequeue.Capability]bool {
	return corequeue.CloneCapabilities(capabilities())
}

func (q *Queue) ListQueues(context.Context) ([]*corequeue.QueueInfo, error) {
	cfg := q.cfg.Normalize()
	out := make([]*corequeue.QueueInfo, 0, len(cfg.Queues))
	for name, priority := range cfg.Queues {
		out = append(out, &corequeue.QueueInfo{Name: name, State: corequeue.QueueRunning, Priority: priority, UpdatedAt: time.Now()})
	}
	return out, nil
}

func (q *Queue) GetQueue(_ context.Context, queueName string) (*corequeue.QueueInfo, error) {
	cfg := q.cfg.Normalize()
	priority := cfg.Queues[queueName]
	return &corequeue.QueueInfo{Name: queueName, State: corequeue.QueueRunning, Priority: priority, UpdatedAt: time.Now()}, nil
}

func (q *Queue) ListTasks(context.Context, corequeue.TaskQuery) ([]*corequeue.TaskInfo, error) {
	return nil, unsupported(corequeue.CapInspector)
}

func (q *Queue) GetTask(context.Context, string, string) (*corequeue.TaskInfo, error) {
	return nil, unsupported(corequeue.CapInspector)
}

func (q *Queue) DeleteTask(context.Context, string, string) error {
	return unsupported(corequeue.CapCancel)
}

func (q *Queue) RetryTask(context.Context, string, string) error {
	return unsupported(corequeue.CapInspector)
}

func (q *Queue) ArchiveTask(context.Context, string, string) error {
	return unsupported(corequeue.CapInspector)
}

func (q *Queue) CancelTask(context.Context, string, string) error {
	return unsupported(corequeue.CapCancel)
}

func (q *Queue) PauseQueue(context.Context, string) error {
	return unsupported(corequeue.CapPauseResume)
}

func (q *Queue) ResumeQueue(context.Context, string) error {
	return unsupported(corequeue.CapPauseResume)
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

func maxRetryValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
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

var _ corequeue.QueueRunner = (*Queue)(nil)
