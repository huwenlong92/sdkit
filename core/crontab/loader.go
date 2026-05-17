package crontab

import (
	"context"
	"fmt"
	"strings"
)

type Loader interface {
	Load(ctx context.Context) ([]Entry, error)
}

type BuiltinLoader struct {
	Entries []Entry
}

func (l BuiltinLoader) Load(ctx context.Context) ([]Entry, error) {
	entries := make([]Entry, 0, len(l.Entries))
	for _, entry := range l.Entries {
		entry.Source = SourceBuiltin
		if entry.ID == "" {
			entry.ID = "builtin." + entry.TemplateKey
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

type DatabaseLoader struct {
	Repo EntryRepository
}

func (l DatabaseLoader) Load(ctx context.Context) ([]Entry, error) {
	if l.Repo == nil {
		return nil, nil
	}
	entries, err := l.Repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Source = SourceDB
		if strings.TrimSpace(entries[i].ID) == "" {
			return nil, fmt.Errorf("crontab db entry id is required")
		}
		if entries[i].ID != "" && !strings.HasPrefix(entries[i].ID, "db.") {
			entries[i].ID = "db." + entries[i].ID
		}
	}
	return entries, nil
}
