package crontab

import "time"

type Entry struct {
	ID          string
	Name        string
	TemplateKey string

	Spec    string
	Payload string
	Source  Source

	Enabled bool

	Timeout time.Duration

	Distributed bool
	LockTTL     time.Duration
	LockKey     string

	MaxRunCount int64
}

type EntryInfo struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	TemplateKey  string        `json:"template_key"`
	Spec         string        `json:"spec"`
	Payload      string        `json:"payload"`
	Source       Source        `json:"source"`
	Mode         Mode          `json:"mode"`
	Enabled      bool          `json:"enabled"`
	Timeout      time.Duration `json:"timeout"`
	Distributed  bool          `json:"distributed"`
	LockTTL      time.Duration `json:"lock_ttl"`
	LockKey      string        `json:"lock_key"`
	LastStatus   Status        `json:"last_status"`
	LastRunAt    int64         `json:"last_run_at"`
	NextRunAt    int64         `json:"next_run_at"`
	LastError    string        `json:"last_error"`
	RunCount     int64         `json:"run_count"`
	MaxRunCount  int64         `json:"max_run_count"`
	SuccessCount int64         `json:"success_count"`
	FailCount    int64         `json:"fail_count"`
	SkipCount    int64         `json:"skip_count"`
}

type CreateEntryRequest struct {
	Name        string
	TemplateKey string
	Spec        string
	Payload     string
	Enabled     bool
	Timeout     time.Duration
	Distributed bool
	LockTTL     time.Duration
	LockKey     string
	MaxRunCount int64
}

type UpdateEntryRequest struct {
	Name        string
	TemplateKey string
	Spec        string
	Payload     string
	Enabled     bool
	Timeout     time.Duration
	Distributed bool
	LockTTL     time.Duration
	LockKey     string
	MaxRunCount int64
}

type RunOnceRequest struct {
	EntryID     string
	TemplateKey string
	Payload     string
	Timeout     time.Duration
	LockKey     string
}

func EntryToJob(e Entry) Job {
	name := e.TemplateKey
	if name == "" {
		name = e.Name
	}
	return Job{
		ID:          e.ID,
		Name:        name,
		Label:       e.Name,
		Spec:        e.Spec,
		Source:      e.Source,
		Mode:        ModeLocal,
		Enabled:     e.Enabled,
		Payload:     e.Payload,
		Timeout:     e.Timeout,
		Distributed: e.Distributed,
		LockTTL:     e.LockTTL,
		LockKey:     e.LockKey,
		MaxRunCount: e.MaxRunCount,
	}
}

func JobToEntry(j Job) Entry {
	source := j.Source
	if source == SourceDynamic {
		source = SourceDB
	}
	return Entry{
		ID:          j.ID,
		Name:        j.Label,
		TemplateKey: j.Name,
		Spec:        j.Spec,
		Payload:     j.Payload,
		Source:      source,
		Enabled:     j.Enabled,
		Timeout:     j.Timeout,
		Distributed: j.Distributed,
		LockTTL:     j.LockTTL,
		LockKey:     j.LockKey,
		MaxRunCount: j.MaxRunCount,
	}
}

func entryInfo(e Entry) EntryInfo {
	return EntryInfo{
		ID:          e.ID,
		Name:        e.Name,
		TemplateKey: e.TemplateKey,
		Spec:        e.Spec,
		Payload:     e.Payload,
		Source:      e.Source,
		Enabled:     e.Enabled,
		Timeout:     e.Timeout,
		Distributed: e.Distributed,
		LockTTL:     e.LockTTL,
		LockKey:     e.LockKey,
		MaxRunCount: e.MaxRunCount,
	}
}
