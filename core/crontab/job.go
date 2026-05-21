package crontab

import "time"

type Job struct {
	ID     string
	Name   string
	Label  string
	Spec   string
	Source Source
	Mode   Mode

	Enabled bool

	Payload  string
	Queue    string
	TaskType string

	Timeout     time.Duration
	Distributed bool
	LockTTL     time.Duration
	LockKey     string

	MaxRunCount int64
}

type JobStatus struct {
	ID          string
	Name        string
	Label       string
	Source      Source
	Mode        Mode
	Spec        string
	Enabled     bool
	Running     bool
	LastStatus  Status
	LastRunAt   int64
	NextRunAt   int64
	RunCount    int64
	MaxRunCount int64
	FailCount   int64
	LastError   string
}

type RunLog struct {
	JobID   string
	JobName string
	RunID   string

	Source Source
	Mode   Mode
	Spec   string

	Queue    string
	TaskType string
	Payload  string
	TrackID  string

	Status     Status
	StartedAt  int64
	FinishedAt int64
	DurationMs int64

	Error      string
	PanicStack string

	Host       string
	InstanceID string
}
