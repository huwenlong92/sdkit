package crontab

import "context"

type Scheduler interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Reload(ctx context.Context, jobs []Job) error
}
