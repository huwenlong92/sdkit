package crontab

type Mode string

const (
	ModeLocal Mode = "local"
)

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceDynamic Source = "dynamic"
	SourceDB      Source = "db"
	SourceManual  Source = "manual"
)

type Status string

const (
	StatusPending          Status = "pending"
	StatusRunning          Status = "running"
	StatusSuccess          Status = "success"
	StatusFailed           Status = "failed"
	StatusSkipped          Status = "skipped"
	StatusTimeout          Status = "timeout"
	StatusPanic            Status = "panic"
	StatusLocked           Status = "locked"
	StatusDisabled         Status = "disabled"
	StatusTemplateMissing  Status = "template_missing"
	StatusTemplateDisabled Status = "template_disabled"
)
