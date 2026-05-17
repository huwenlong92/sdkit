package crontab

import (
	"context"
	"testing"
)

type loaderEntryRepository struct {
	entries []Entry
}

func (r loaderEntryRepository) ListEnabled(ctx context.Context) ([]Entry, error) {
	return r.entries, nil
}

func (r loaderEntryRepository) ListEntries(ctx context.Context) ([]EntryInfo, error) {
	return nil, nil
}

func (r loaderEntryRepository) GetEntry(ctx context.Context, id string) (EntryInfo, error) {
	return EntryInfo{}, nil
}

func (r loaderEntryRepository) Create(ctx context.Context, entry Entry) (EntryInfo, error) {
	return entryInfo(entry), nil
}

func (r loaderEntryRepository) Update(ctx context.Context, entry Entry) error {
	return nil
}

func (r loaderEntryRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func (r loaderEntryRepository) UpdateEnabled(ctx context.Context, id string, enabled bool) error {
	return nil
}

func (r loaderEntryRepository) ListRuns(ctx context.Context, filter RunFilter) ([]RunLog, error) {
	return nil, nil
}

func TestDatabaseLoaderRequiresAndPrefixesDBID(t *testing.T) {
	loader := DatabaseLoader{Repo: loaderEntryRepository{entries: []Entry{{ID: "123", TemplateKey: "cleanup"}}}}
	entries, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "db.123" || entries[0].Source != SourceDB {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	loader = DatabaseLoader{Repo: loaderEntryRepository{entries: []Entry{{TemplateKey: "cleanup"}}}}
	if _, err := loader.Load(context.Background()); err == nil {
		t.Fatal("expected missing db entry id error")
	}
}
