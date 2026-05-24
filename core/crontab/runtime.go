package crontab

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/huwenlong92/sdkit/core/logger"
	"github.com/huwenlong92/sdkit/core/tracking"

	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type entryContextKey struct{}
type runContextKey struct{}

func ContextWithEntry(ctx context.Context, entry Entry) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, entryContextKey{}, entry)
}

func EntryFromContext(ctx context.Context) (Entry, bool) {
	if ctx == nil {
		return Entry{}, false
	}
	entry, ok := ctx.Value(entryContextKey{}).(Entry)
	return entry, ok
}

func ContextWithRunContext(ctx context.Context, run *RunContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runContextKey{}, run)
}

func RunContextFromContext(ctx context.Context) (*RunContext, bool) {
	if ctx == nil {
		return nil, false
	}
	run, ok := ctx.Value(runContextKey{}).(*RunContext)
	return run, ok && run != nil
}

type Runtime struct {
	registry *Registry
}

func NewRuntime(registry *Registry) *Runtime {
	if registry == nil {
		registry = defaultRegistry
	}
	return &Runtime{registry: registry}
}

var (
	defaultRegistry = NewRegistry()
	defaultRuntime  = NewRuntime(defaultRegistry)
)

func Register(templates ...Template) error {
	return defaultRegistry.RegisterAll(templates...)
}

func MustRegister(templates ...Template) {
	for _, template := range templates {
		defaultRegistry.MustRegister(template)
	}
}

func Dispatch(ctx context.Context, entry *Entry) error {
	return defaultRuntime.Dispatch(ctx, entry)
}

func (rt *Runtime) Dispatch(ctx context.Context, entry *Entry) error {
	if rt == nil || rt.registry == nil {
		return ErrRegistryRequired
	}
	return rt.registry.Dispatch(ctx, entry)
}

func (r *Registry) Dispatch(ctx context.Context, entry *Entry) error {
	ctx = normalizeContext(ctx)
	startedAt := time.Now()
	runCtx, _ := RunContextFromContext(ctx)
	if runCtx != nil {
		startedAt = runCtx.StartedAt
	}

	entryValue := normalizeEntry(entry)
	name := entryTemplateKey(&entryValue)
	ctx = ensureRuntimeTrackID(ctx, runCtx)

	ctx, span := startExecuteSpan(ctx, entryValue, name, entryValue.Spec, false, 0)
	defer span.End()

	result := RunResult{Status: StatusSuccess}
	runningWritten := false
	var tpl Template
	defer func() {
		result = result.normalize()
		finishedAt := time.Now()
		duration := finishedAt.Sub(startedAt)
		finishRunSpan(span, result.Status, result.Err, duration)
		recordRuntimeExecution(result.Status, duration)
		if runCtx != nil {
			runCtx.Result = result
		}
		finishRuntime(runCtx, ctx, entryValue, tpl, result, startedAt, finishedAt, runningWritten)
		logRuntimeResult(ctx, runCtx, entryValue, tpl, result, duration)
		notifyFailure(ctx, entryValue, tpl, startedAt, finishedAt, duration, result)
	}()

	if r == nil {
		result = RunResult{Status: StatusFailed, Err: ErrRegistryRequired}
		return runtimeResultError(result)
	}
	if name == "" {
		result = RunResult{Status: StatusTemplateMissing, Err: ErrTemplateNotFound}
		return runtimeResultError(result)
	}
	job := EntryToJob(entryValue)
	job.Name = name
	var err error
	tpl, err = r.ValidateJob(job)
	if err != nil {
		result = RunResult{Status: validateStatus(err), Err: err}
		return runtimeResultError(result)
	}
	if job.Spec == "" {
		job.Spec = tpl.Spec
	}
	if job.Label == "" {
		job.Label = tpl.Name
	}
	if !job.Enabled {
		result = RunResult{Status: StatusDisabled}
		return runtimeResultError(result)
	}
	if runCtx == nil {
		runCtx = newRunContext(ctx, nil, job)
		startedAt = runCtx.StartedAt
		ctx = runCtx.Context()
	}

	timeout := runtimeTimeout(tpl, job)
	entryValue = JobToEntry(job)
	if entryValue.ID == "" && entry != nil {
		entryValue.ID = entry.ID
	}
	setExecuteSpanTemplate(span, tpl.Name, job.Spec, tpl.AllowOverlap, timeout)
	prepareRunContext(runCtx, ctx, tpl, job, entryValue)
	ctx = runCtx.Context()

	unlock, lockErr := acquireRuntimeLock(ctx, runCtx, entryValue, tpl, job, timeout)
	if lockErr != nil {
		result = lockErr.result
		return runtimeResultError(result)
	}
	defer unlock()

	logRuntimeStart(ctx, runCtx, entryValue, tpl)
	if err := writeRuntimeRunning(runCtx, ctx, tpl); err != nil {
		result = runResultFromMarkRunningError(err)
		return runtimeResultError(result)
	}
	runningWritten = true

	ctx = injectRuntimeJobLogger(ctx, runCtx, entryValue, job, tpl)
	result = runRuntimeHandler(ctx, runCtx, tpl, job, timeout)
	return runtimeResultError(result)
}

func entryTemplateKey(entry *Entry) string {
	if entry == nil {
		return ""
	}
	if entry.TemplateKey != "" {
		return entry.TemplateKey
	}
	return entry.Name
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func normalizeEntry(entry *Entry) Entry {
	if entry == nil {
		return Entry{}
	}
	return *entry
}

type runtimeError struct {
	result RunResult
}

func (e *runtimeError) Error() string {
	if e == nil {
		return ""
	}
	if e.result.Err != nil {
		return e.result.Err.Error()
	}
	return string(e.result.Status)
}

func (e *runtimeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.result.Err
}

func runtimeResultError(result RunResult) error {
	result = result.normalize()
	if result.Status == StatusSuccess {
		return nil
	}
	return &runtimeError{result: result}
}

func runResultFromError(err error) RunResult {
	if err == nil {
		return RunResult{Status: StatusSuccess}
	}
	var runtimeErr *runtimeError
	if errors.As(err, &runtimeErr) {
		return runtimeErr.result.normalize()
	}
	return RunResult{Status: StatusFailed, Err: err}
}

func ensureRuntimeTrackID(ctx context.Context, runCtx *RunContext) context.Context {
	if tracking.TrackID(ctx) == "" {
		ctx = tracking.WithTrackID(ctx, tracking.NewTrackID())
	}
	if runCtx != nil {
		runCtx.SetContext(ctx)
	}
	return ctx
}

func templateTimeout(tpl Template) time.Duration {
	if tpl.Timeout > 0 {
		return tpl.Timeout
	}
	return 0
}

func runtimeTimeout(tpl Template, job Job) time.Duration {
	if job.Timeout > 0 {
		return job.Timeout
	}
	return templateTimeout(tpl)
}

func prepareRunContext(runCtx *RunContext, ctx context.Context, tpl Template, job Job, entry Entry) {
	if runCtx == nil {
		return
	}
	runCtx.Template = tpl
	runCtx.Job = job
	runCtx.Entry = entry
	runCtx.SetContext(ctx)
}

type runtimeLockError struct {
	result RunResult
}

func acquireRuntimeLock(ctx context.Context, runCtx *RunContext, entry Entry, tpl Template, job Job, timeout time.Duration) (func(), *runtimeLockError) {
	if tpl.AllowOverlap {
		return func() {}, nil
	}
	if runCtx == nil || runCtx.runner == nil {
		return func() {}, nil
	}
	if !runCtx.runner.cfg.Lock.Enabled {
		return func() {}, nil
	}

	key := runtimeLockKey(entry)
	ttl := job.LockTTL
	if ttl <= 0 {
		ttl = runCtx.runner.cfg.Lock.TTL
	}
	if ttl <= 0 && timeout > 0 {
		ttl = timeout + 30*time.Second
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	lock, ok, err := runCtx.runner.locker.TryLock(ctx, key, ttl)
	if err != nil {
		return func() {}, &runtimeLockError{result: RunResult{Status: StatusFailed, Err: err}}
	}
	if !ok {
		return func() {}, &runtimeLockError{result: RunResult{Status: StatusLocked, Err: ErrJobRunning}}
	}
	if lock == nil {
		return func() {}, nil
	}
	stopRefresh := runCtx.runner.refreshLock(ctx, lock, ttl)
	return func() {
		stopRefresh()
		_ = lock.Unlock(ctx)
	}, nil
}

func runtimeLockKey(entry Entry) string {
	id := entry.ID
	if id == "" {
		id = entry.TemplateKey
	}
	if id == "" {
		id = entry.Name
	}
	return "crontab:entry:" + id
}

func writeRuntimeRunning(runCtx *RunContext, ctx context.Context, tpl Template) error {
	if runCtx == nil || runCtx.runner == nil {
		return nil
	}
	if !tpl.LogDisabled {
		runLog := runCtx.runningLog()
		if err := runCtx.runner.store.MarkRunning(ctx, runLog); err != nil {
			return err
		}
		_ = runCtx.runner.logger.Write(ctx, runLog)
	}
	if runCtx.runner.runtime != nil {
		runCtx.runner.runtime.MarkRunning(runCtx.Entry, runCtx.StartedAt)
	}
	return nil
}

func runResultFromMarkRunningError(err error) RunResult {
	if errors.Is(err, ErrRunLimitReached) {
		return RunResult{Status: StatusDisabled, Err: err}
	}
	return RunResult{Status: StatusFailed, Err: err}
}

func injectRuntimeJobLogger(ctx context.Context, runCtx *RunContext, entry Entry, job Job, tpl Template) context.Context {
	if runCtx == nil || runCtx.runner == nil {
		return ctx
	}
	if tpl.LogDisabled {
		ctx = WithJobLogger(ctx, NoopJobLogger{})
		runCtx.SetContext(ctx)
		return ctx
	}
	runLogger := &runJobLogger{
		runID:       runCtx.RunID,
		entryID:     entry.ID,
		templateKey: job.Name,
		store:       runCtx.runner.logStore,
		streamer:    runCtx.runner.streamer,
	}
	ctx = WithJobLogger(ctx, runLogger)
	runLogger.ctx = ctx
	runCtx.SetContext(ctx)
	return ctx
}

func runRuntimeHandler(ctx context.Context, runCtx *RunContext, tpl Template, job Job, timeout time.Duration) RunResult {
	handler := func(c *RunContext) RunResult {
		if err := executeTemplateHandler(c.Context(), c, tpl, job); err != nil {
			return runResultFromError(err)
		}
		return RunResult{Status: StatusSuccess}
	}
	if timeout <= 0 {
		runCtx.SetContext(ctx)
		return runHandler(runCtx, handler)
	}

	runCtxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan RunResult, 1)
	go func() {
		child := runCtx.clone()
		child.SetContext(runCtxWithTimeout)
		done <- runHandler(child, handler)
	}()

	select {
	case <-runCtxWithTimeout.Done():
		return RunResult{Status: StatusTimeout, Err: runCtxWithTimeout.Err()}
	case result := <-done:
		result = result.normalize()
		if errors.Is(result.Err, context.DeadlineExceeded) && runCtxWithTimeout.Err() != nil {
			return RunResult{Status: StatusTimeout, Err: runCtxWithTimeout.Err()}
		}
		return result
	}
}

func runHandler(runCtx *RunContext, handler RunHandler) (result RunResult) {
	defer func() {
		if rec := recover(); rec != nil {
			err := fmt.Errorf("panic: %v\n%s", rec, string(debug.Stack()))
			result = RunResult{Status: StatusPanic, Err: err, PanicStack: err.Error()}
		}
	}()
	return handler(runCtx).normalize()
}

func executeTemplateHandler(ctx context.Context, runCtx *RunContext, tpl Template, job Job) error {
	if tpl.Handler == nil {
		return ErrLocalHandlerMissing
	}
	entry := JobToEntry(job)

	child := runCtx.clone()
	child.Template = tpl
	child.Job = job
	child.Entry = entry
	child.SetContext(ContextWithEntry(ctx, entry))

	result := tpl.Handler(child).normalize()
	if result.Status != StatusSuccess {
		return runtimeResultError(result)
	}
	return nil
}

func finishRuntime(runCtx *RunContext, ctx context.Context, entry Entry, tpl Template, result RunResult, startedAt time.Time, finishedAt time.Time, runningWritten bool) {
	if runCtx == nil || runCtx.runner == nil {
		return
	}
	if !runningWritten && errors.Is(result.Err, ErrRunLimitReached) {
		return
	}
	if !runningWritten {
		runCtx.Entry = entry
	}
	if !tpl.LogDisabled {
		runLog := runCtx.finishedLog(result.normalize(), finishedAt)
		_ = runCtx.runner.logger.Write(ctx, runLog)
		_ = runCtx.runner.store.MarkFinished(ctx, runLog)
	}
	if runCtx.runner.runtime != nil {
		runCtx.runner.runtime.MarkFinished(entry, runtimeStatus(result.Status), startedAt, finishedAt, result.Err)
	}
}

func runtimeLogger(runCtx *RunContext) *zap.Logger {
	if runCtx == nil || runCtx.runner == nil {
		return nil
	}
	return runCtx.runner.runtimeLogger
}

func logRuntimeStart(ctx context.Context, runCtx *RunContext, entry Entry, tpl Template) {
	log := runtimeLogger(runCtx)
	if log == nil || tpl.LogDisabled {
		return
	}
	log = logger.WithContext(ctx, log)
	log.Info("crontab execute start", runtimeLogFields(runCtx, entry, tpl, RunResult{Status: StatusRunning}, 0)...)
}

func logRuntimeResult(ctx context.Context, runCtx *RunContext, entry Entry, tpl Template, result RunResult, duration time.Duration) {
	log := runtimeLogger(runCtx)
	if log == nil || tpl.LogDisabled {
		return
	}
	result = result.normalize()
	fields := runtimeLogFields(runCtx, entry, tpl, result, duration)
	log = logger.WithContext(ctx, log)
	switch result.Status {
	case StatusSuccess:
		log.Debug("crontab execute success", fields...)
	case StatusLocked:
		log.Warn("crontab overlap skipped", fields...)
	case StatusDisabled, StatusSkipped, StatusTemplateDisabled, StatusTemplateMissing:
		log.Warn("crontab execute skipped", fields...)
	case StatusTimeout:
		log.Error("crontab execute timeout", append(fields, zap.Error(runtimeLogError(result)))...)
	default:
		log.Error("crontab execute failed", append(fields, zap.Error(runtimeLogError(result)))...)
	}
}

func runtimeLogFields(runCtx *RunContext, entry Entry, tpl Template, result RunResult, duration time.Duration) []zap.Field {
	templateKey := tpl.Key
	if templateKey == "" {
		templateKey = entry.TemplateKey
	}
	if templateKey == "" {
		templateKey = entry.Name
	}
	fields := []zap.Field{
		zap.String("template_key", templateKey),
		zap.String("entry_id", entry.ID),
		zap.String("status", string(result.Status)),
	}
	if tpl.Name != "" {
		fields = append(fields, zap.String("template_name", tpl.Name))
	}
	if runCtx != nil && runCtx.RunID != "" {
		fields = append(fields, zap.String("run_id", runCtx.RunID))
	}
	if duration > 0 {
		fields = append(fields, zap.Int64("duration_ms", duration.Milliseconds()))
	}
	return fields
}

func runtimeLogError(result RunResult) error {
	if result.Err != nil {
		return result.Err
	}
	return fmt.Errorf("crontab runtime status: %s", result.Status)
}

func notifyFailure(ctx context.Context, entry Entry, tpl Template, startedAt, finishedAt time.Time, duration time.Duration, result RunResult) {
	if !isFailureStatus(result.Status) {
		return
	}
	report := FailureReport{
		EntryID:     entry.ID,
		TemplateKey: failureTemplateKey(entry, tpl),
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
		Duration:    duration,
		TraceID:     traceIDFromContext(ctx),
		Error:       runtimeLogError(result),
	}
	runFailureHandlers(ctx, report)
}

func failureTemplateKey(entry Entry, tpl Template) string {
	if tpl.Key != "" {
		return tpl.Key
	}
	if entry.TemplateKey != "" {
		return entry.TemplateKey
	}
	return entry.Name
}

func traceIDFromContext(ctx context.Context) string {
	spanContext := oteltrace.SpanContextFromContext(ctx)
	if spanContext.IsValid() && spanContext.HasTraceID() {
		return spanContext.TraceID().String()
	}
	return logger.Field(ctx, logger.TraceIDKey)
}

func isFailureStatus(status Status) bool {
	switch status {
	case StatusFailed, StatusTimeout, StatusPanic:
		return true
	default:
		return false
	}
}

type RuntimeStatus string

const (
	RuntimeWaiting  RuntimeStatus = "waiting"
	RuntimeRunning  RuntimeStatus = "running"
	RuntimeSuccess  RuntimeStatus = "success"
	RuntimeFailed   RuntimeStatus = "failed"
	RuntimeSkipped  RuntimeStatus = "skipped"
	RuntimeDisabled RuntimeStatus = "disabled"
)

type RuntimeInfo struct {
	EntryID      string        `json:"entry_id"`
	TemplateKey  string        `json:"template_key"`
	Status       RuntimeStatus `json:"status"`
	PrevTime     time.Time     `json:"prev_time"`
	NextTime     time.Time     `json:"next_time"`
	LastStartAt  time.Time     `json:"last_start_at"`
	LastEndAt    time.Time     `json:"last_end_at"`
	LastDuration time.Duration `json:"last_duration"`
	LastError    string        `json:"last_error"`
	RunCount     int64         `json:"run_count"`
	SuccessCount int64         `json:"success_count"`
	FailCount    int64         `json:"fail_count"`
	SkipCount    int64         `json:"skip_count"`
}

type RuntimeState struct {
	mu    sync.RWMutex
	items map[string]RuntimeInfo
}

func NewRuntimeState() *RuntimeState {
	return &RuntimeState{items: make(map[string]RuntimeInfo)}
}

func (s *RuntimeState) SetSchedule(entries []Entry) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]RuntimeInfo, len(entries))
	for _, entry := range entries {
		info := s.items[entry.ID]
		info.EntryID = entry.ID
		info.TemplateKey = entry.TemplateKey
		if !entry.Enabled {
			info.Status = RuntimeDisabled
		} else if info.Status == "" {
			info.Status = RuntimeWaiting
		}
		next[entry.ID] = info
	}
	s.items = next
}

func (s *RuntimeState) MarkRunning(entry Entry, started time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.items[entry.ID]
	info.EntryID = entry.ID
	info.TemplateKey = entry.TemplateKey
	info.Status = RuntimeRunning
	info.LastStartAt = started
	info.LastError = ""
	s.items[entry.ID] = info
}

func (s *RuntimeState) MarkFinished(entry Entry, status RuntimeStatus, started, ended time.Time, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.items[entry.ID]
	info.EntryID = entry.ID
	info.TemplateKey = entry.TemplateKey
	info.Status = status
	info.LastStartAt = started
	info.LastEndAt = ended
	info.LastDuration = ended.Sub(started)
	info.RunCount++
	if err != nil {
		info.LastError = err.Error()
	}
	switch status {
	case RuntimeSuccess:
		info.SuccessCount++
	case RuntimeFailed:
		info.FailCount++
	case RuntimeSkipped:
		info.SkipCount++
	}
	s.items[entry.ID] = info
}

func (s *RuntimeState) List() []RuntimeInfo {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RuntimeInfo, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	return out
}

func (s *RuntimeState) Get(id string) (RuntimeInfo, bool) {
	if s == nil {
		return RuntimeInfo{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	return item, ok
}
