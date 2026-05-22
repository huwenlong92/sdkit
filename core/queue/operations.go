package queue

import (
	"context"
	"errors"
	"sync"
	"time"
)

type QueueDrainer interface {
	DrainQueue(ctx context.Context, queue string) error
}

type OperationsRuntime struct {
	mu        sync.RWMutex
	manager   Manager
	taskStore TaskStore
	metadata  RuntimeMetadata
	draining  map[string]bool
}

func NewOperationsRuntime(manager Manager) *OperationsRuntime {
	return &OperationsRuntime{manager: manager}
}

func (o *OperationsRuntime) SetManager(manager Manager) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.manager = manager
	o.mu.Unlock()
}

func (o *OperationsRuntime) SetTaskStore(store TaskStore) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.taskStore = store
	o.mu.Unlock()
}

func (o *OperationsRuntime) SetMetadata(metadata RuntimeMetadata) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.metadata = cloneRuntimeMetadata(metadata)
	o.mu.Unlock()
}

func (o *OperationsRuntime) Manager() Manager {
	if o == nil {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.manager
}

func (o *OperationsRuntime) TaskStore() TaskStore {
	if o == nil {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.taskStore
}

func (o *OperationsRuntime) Metadata() RuntimeMetadata {
	if o == nil {
		return RuntimeMetadata{}
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return cloneRuntimeMetadata(o.metadata)
}

func (o *OperationsRuntime) Supports(cap Capability) bool {
	manager := o.Manager()
	if manager == nil {
		return false
	}
	return manager.Supports(cap)
}

func (o *OperationsRuntime) Capabilities() map[Capability]bool {
	manager := o.Manager()
	if manager == nil {
		return nil
	}
	return manager.Capabilities()
}

func (o *OperationsRuntime) Status(ctx context.Context) ([]*QueueInfo, error) {
	return o.ListQueues(ctx)
}

func (o *OperationsRuntime) RuntimeStatus(ctx context.Context) (*RuntimeStatusInfo, error) {
	queues, err := o.QueueStatus(ctx)
	if err != nil {
		return nil, err
	}
	worker := o.WorkerStatus(ctx)
	state := runtimeStateFromQueues(queues)
	if worker.State == RuntimeStopped {
		state = RuntimeStopped
	}
	return &RuntimeStatusInfo{
		State:     state,
		Queues:    queues,
		Worker:    worker,
		UpdatedAt: time.Now(),
	}, nil
}

func (o *OperationsRuntime) QueueStatus(ctx context.Context) ([]QueueRuntimeStatus, error) {
	queues, err := o.ListQueues(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]QueueRuntimeStatus, 0, len(queues))
	for _, queue := range queues {
		if queue == nil {
			continue
		}
		state := queue.State
		if o.isDraining(queue.Name) {
			state = QueueDraining
		}
		out = append(out, QueueRuntimeStatus{
			Name:      queue.Name,
			State:     state,
			Priority:  queue.Priority,
			Metrics:   queueMetricsFromInfo(queue),
			PausedAt:  queue.PausedAt,
			UpdatedAt: queue.UpdatedAt,
		})
	}
	return out, nil
}

func (o *OperationsRuntime) WorkerStatus(ctx context.Context) WorkerRuntimeStatus {
	metadata := o.Metadata()
	state := RuntimeRunning
	if o.Manager() == nil {
		state = RuntimeStopped
	}
	return WorkerRuntimeStatus{
		ID:             metadata.WorkerMetadata.ID,
		Name:           firstNonEmpty(metadata.WorkerMetadata.Name, metadata.Worker, metadata.Name),
		Service:        firstNonEmpty(metadata.WorkerMetadata.Service, metadata.Service),
		State:          state,
		Queues:         cloneQueueWeights(firstQueueWeights(metadata.WorkerMetadata.Queues, metadata.Queues)),
		StrictPriority: metadata.StrictPriority,
		Concurrency:    metadata.Concurrency,
		UpdatedAt:      time.Now(),
	}
}

func (o *OperationsRuntime) Metrics(ctx context.Context) (*RuntimeMetrics, error) {
	queues, err := o.ListQueues(ctx)
	if err != nil {
		return nil, err
	}
	metrics := &RuntimeMetrics{
		Queues: map[string]QueueMetrics{},
	}
	for _, queue := range queues {
		if queue == nil {
			continue
		}
		item := queueMetricsFromInfo(queue)
		metrics.Queues[queue.Name] = item
		metrics.Total = addQueueMetrics(metrics.Total, item)
	}
	return metrics, nil
}

func (o *OperationsRuntime) ListQueues(ctx context.Context) ([]*QueueInfo, error) {
	manager := o.Manager()
	if manager == nil {
		return nil, ErrNotInitialized
	}
	return manager.ListQueues(ctx)
}

func (o *OperationsRuntime) GetQueue(ctx context.Context, queueName string) (*QueueInfo, error) {
	manager := o.Manager()
	if manager == nil {
		return nil, ErrNotInitialized
	}
	return manager.GetQueue(ctx, queueName)
}

func (o *OperationsRuntime) ListTasks(ctx context.Context, query TaskQuery) ([]*TaskInfo, error) {
	manager := o.Manager()
	if manager == nil {
		return nil, ErrNotInitialized
	}
	return manager.ListTasks(ctx, query)
}

func (o *OperationsRuntime) GetTask(ctx context.Context, queueName string, taskID string) (*TaskInfo, error) {
	manager := o.Manager()
	if manager == nil {
		return nil, ErrNotInitialized
	}
	return manager.GetTask(ctx, queueName, taskID)
}

func (o *OperationsRuntime) DeleteTask(ctx context.Context, queueName string, taskID string) error {
	manager := o.Manager()
	if manager == nil {
		return ErrNotInitialized
	}
	err := manager.DeleteTask(ctx, queueName, taskID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrTaskNotFound) && !errors.Is(err, ErrCapabilityUnsupported) {
		return err
	}
	store, ok := o.TaskStore().(TaskDeletionStore)
	if !ok || store == nil {
		return err
	}
	if storeErr := store.DeleteTaskRecord(ctx, queueName, taskID); storeErr != nil {
		if errors.Is(storeErr, ErrTaskNotFound) {
			return err
		}
		return storeErr
	}
	return nil
}

func (o *OperationsRuntime) RetryTask(ctx context.Context, queueName string, taskID string) error {
	manager := o.Manager()
	if manager == nil {
		return ErrNotInitialized
	}
	return manager.RetryTask(ctx, queueName, taskID)
}

func (o *OperationsRuntime) ArchiveTask(ctx context.Context, queueName string, taskID string) error {
	manager := o.Manager()
	if manager == nil {
		return ErrNotInitialized
	}
	return manager.ArchiveTask(ctx, queueName, taskID)
}

func (o *OperationsRuntime) CancelTask(ctx context.Context, queueName string, taskID string) error {
	manager := o.Manager()
	if manager == nil {
		return ErrNotInitialized
	}
	return manager.CancelTask(ctx, queueName, taskID)
}

func (o *OperationsRuntime) PauseQueue(ctx context.Context, queueName string) error {
	manager := o.Manager()
	if manager == nil {
		return ErrNotInitialized
	}
	return manager.PauseQueue(ctx, queueName)
}

func (o *OperationsRuntime) ResumeQueue(ctx context.Context, queueName string) error {
	manager := o.Manager()
	if manager == nil {
		return ErrNotInitialized
	}
	if err := manager.ResumeQueue(ctx, queueName); err != nil {
		return err
	}
	o.setDraining(queueName, false)
	return nil
}

func (o *OperationsRuntime) DrainQueue(ctx context.Context, queueName string) error {
	manager := o.Manager()
	if manager == nil {
		return ErrNotInitialized
	}
	if drainer, ok := manager.(QueueDrainer); ok {
		if err := drainer.DrainQueue(ctx, queueName); err != nil {
			return err
		}
		o.setDraining(queueName, true)
		return nil
	}
	if err := manager.PauseQueue(ctx, queueName); err != nil {
		return err
	}
	o.setDraining(queueName, true)
	return nil
}

func (o *OperationsRuntime) Drain(ctx context.Context) error {
	queues, err := o.ListQueues(ctx)
	if err != nil {
		return err
	}
	for _, queue := range queues {
		if queue == nil {
			continue
		}
		if err := o.DrainQueue(ctx, queue.Name); err != nil {
			return err
		}
	}
	return nil
}

func (o *OperationsRuntime) ListFailedTasks(ctx context.Context, query TaskQuery) ([]*TaskInfo, error) {
	query.State = StateFailed
	tasks, err := o.ListTasks(ctx, query)
	if err == nil {
		return tasks, nil
	}
	if !errors.Is(err, ErrCapabilityUnsupported) {
		return nil, err
	}
	query.State = StateArchived
	return o.ListTasks(ctx, query)
}

func (o *OperationsRuntime) CleanTasks(ctx context.Context, query TaskQuery) (int, error) {
	tasks, err := o.ListTasks(ctx, query)
	if err != nil {
		return 0, err
	}
	cleaned := 0
	for _, task := range tasks {
		if task == nil {
			continue
		}
		queueName := task.Queue
		if queueName == "" {
			queueName = query.Queue
		}
		if queueName == "" {
			queueName = DefaultQueueName
		}
		if err := o.DeleteTask(ctx, queueName, task.ID); err != nil {
			return cleaned, err
		}
		cleaned++
	}
	return cleaned, nil
}

func (o *OperationsRuntime) setDraining(queueName string, draining bool) {
	if o == nil || queueName == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.draining == nil {
		o.draining = map[string]bool{}
	}
	if !draining {
		delete(o.draining, queueName)
		return
	}
	o.draining[queueName] = true
}

func (o *OperationsRuntime) isDraining(queueName string) bool {
	if o == nil || queueName == "" {
		return false
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.draining[queueName]
}

func runtimeStateFromQueues(queues []QueueRuntimeStatus) RuntimeState {
	if len(queues) == 0 {
		return RuntimeStopped
	}
	state := RuntimeRunning
	for _, queue := range queues {
		switch queue.State {
		case QueueFailed:
			return RuntimeFailed
		case QueueDraining:
			state = RuntimeDraining
		case QueuePaused:
			if state != RuntimeDraining {
				state = RuntimePaused
			}
		}
	}
	return state
}

func firstQueueWeights(first map[string]int, fallback map[string]int) map[string]int {
	if len(first) > 0 {
		return first
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
