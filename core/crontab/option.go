package crontab

import "time"

type Options struct {
	WithSeconds     bool
	AutoLoadDB      bool
	Distributed     bool
	Locker          Locker
	ShutdownTimeout time.Duration
	StrictLoad      bool
}

func DefaultOptions() Options {
	return Options{
		WithSeconds:     true,
		AutoLoadDB:      true,
		Distributed:     false,
		Locker:          NoopLocker{},
		ShutdownTimeout: 30 * time.Second,
		StrictLoad:      true,
	}
}
