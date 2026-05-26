package asynq

import (
	"context"
	"errors"

	"github.com/huwenlong92/sdkit/core/queue"

	hibasynq "github.com/hibiken/asynq"
)

func (q *Queue) ListQueues(ctx context.Context) ([]*queue.QueueInfo, error) {
	if q == nil || q.inspector == nil {
		return nil, queue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	names, err := q.inspector.Queues()
	if err != nil {
		return nil, mapInspectorError(err)
	}
	out := make([]*queue.QueueInfo, 0, len(names))
	for _, name := range names {
		info, err := q.inspector.GetQueueInfo(name)
		if err != nil {
			return nil, mapInspectorError(err)
		}
		out = append(out, fromAsynqQueueInfo(info))
	}
	return out, nil
}

func (q *Queue) GetQueue(ctx context.Context, queueName string) (*queue.QueueInfo, error) {
	if q == nil || q.inspector == nil {
		return nil, queue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := q.inspector.GetQueueInfo(queueName)
	if err != nil {
		return nil, mapInspectorError(err)
	}
	return fromAsynqQueueInfo(info), nil
}

func (q *Queue) ListTasks(ctx context.Context, query queue.TaskQuery) ([]*queue.TaskInfo, error) {
	if q == nil || q.inspector == nil {
		return nil, queue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	queueName := query.Queue
	if queueName == "" {
		queueName = queue.DefaultQueueName
	}
	opts := listOptions(query)
	var (
		tasks []*hibasynq.TaskInfo
		err   error
	)
	switch query.State {
	case "", queue.StatePending:
		tasks, err = q.inspector.ListPendingTasks(queueName, opts...)
	case queue.StateActive:
		tasks, err = q.inspector.ListActiveTasks(queueName, opts...)
	case queue.StateScheduled:
		tasks, err = q.inspector.ListScheduledTasks(queueName, opts...)
	case queue.StateRetry:
		tasks, err = q.inspector.ListRetryTasks(queueName, opts...)
	case queue.StateArchived:
		tasks, err = q.inspector.ListArchivedTasks(queueName, opts...)
	case queue.StateSucceeded:
		tasks, err = q.inspector.ListCompletedTasks(queueName, opts...)
	default:
		return nil, unsupported("asynq", queue.CapInspector)
	}
	if err != nil {
		return nil, mapInspectorError(err)
	}
	out := make([]*queue.TaskInfo, 0, len(tasks))
	for _, task := range tasks {
		info := fromAsynqTaskInfo(task)
		if query.Type != "" && info.Type != query.Type {
			continue
		}
		if query.TaskID != "" && info.ID != query.TaskID {
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

func (q *Queue) GetTask(ctx context.Context, queueName, taskID string) (*queue.TaskInfo, error) {
	if q == nil || q.inspector == nil {
		return nil, queue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := q.inspector.GetTaskInfo(queueName, taskID)
	if err != nil {
		return nil, mapInspectorError(err)
	}
	return fromAsynqTaskInfo(info), nil
}

func (q *Queue) DeleteTask(ctx context.Context, queueName string, taskID string) error {
	if q == nil || q.inspector == nil {
		return queue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return mapInspectorError(q.inspector.DeleteTask(queueName, taskID))
}

func (q *Queue) RetryTask(ctx context.Context, queueName string, taskID string) error {
	if q == nil || q.inspector == nil {
		return queue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return mapInspectorError(q.inspector.RunTask(queueName, taskID))
}

func (q *Queue) ArchiveTask(ctx context.Context, queueName string, taskID string) error {
	if q == nil || q.inspector == nil {
		return queue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return mapInspectorError(q.inspector.ArchiveTask(queueName, taskID))
}

func (q *Queue) CancelTask(ctx context.Context, queueName string, taskID string) error {
	return q.DeleteTask(ctx, queueName, taskID)
}

func (q *Queue) PauseQueue(ctx context.Context, queueName string) error {
	if q == nil || q.inspector == nil {
		return queue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return mapInspectorError(q.inspector.PauseQueue(queueName))
}

func (q *Queue) ResumeQueue(ctx context.Context, queueName string) error {
	if q == nil || q.inspector == nil {
		return queue.ErrNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return mapInspectorError(q.inspector.UnpauseQueue(queueName))
}

func mapInspectorError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, hibasynq.ErrQueueNotFound) {
		return queue.ErrQueueNotFound
	}
	if errors.Is(err, hibasynq.ErrTaskNotFound) {
		return queue.ErrTaskNotFound
	}
	return err
}
