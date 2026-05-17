package crontab

import "time"

type Template struct {
	Key          string
	Name         string
	Desc         string
	Spec         string
	Enabled      bool
	AllowDB      bool
	AllowOverlap bool
	Timeout      time.Duration
	Handler      RunHandler

	DefaultPayload string
	PayloadFormat  string
	PayloadSchema  string
}

type TemplateInfo struct {
	Key             string        `json:"key"`
	Name            string        `json:"name"`
	Label           string        `json:"label"`
	Desc            string        `json:"desc"`
	Description     string        `json:"description"`
	Spec            string        `json:"spec"`
	Mode            string        `json:"mode"`
	Enabled         bool          `json:"enabled"`
	AllowBuiltin    bool          `json:"allow_builtin"`
	AllowDB         bool          `json:"allow_db"`
	AllowOverlap    bool          `json:"allow_overlap"`
	DefaultSpec     string        `json:"default_spec"`
	DefaultPayload  string        `json:"default_payload"`
	DefaultQueue    string        `json:"default_queue"`
	DefaultTaskType string        `json:"default_task_type"`
	PayloadFormat   string        `json:"payload_format"`
	PayloadSchema   string        `json:"payload_schema"`
	Timeout         time.Duration `json:"timeout"`
	TimeoutSec      int           `json:"timeout_sec"`
}
