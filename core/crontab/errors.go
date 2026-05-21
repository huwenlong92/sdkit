package crontab

import "errors"

var (
	ErrRegistryRequired    = errors.New("crontab registry is required")
	ErrTemplateNotFound    = errors.New("crontab template not found")
	ErrTemplateDisabled    = errors.New("crontab template disabled")
	ErrDynamicNotAllowed   = errors.New("crontab template does not allow dynamic")
	ErrEntryNotFound       = errors.New("crontab entry not found")
	ErrLocalHandlerMissing = errors.New("crontab local handler missing")
	ErrJobRunning          = errors.New("crontab job is already running")
	ErrRunLimitReached     = errors.New("crontab run limit reached")
)
