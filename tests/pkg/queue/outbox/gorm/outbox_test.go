package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/queue"
	gormoutbox "github.com/huwenlong92/sdkit/pkg/queue/outbox/gorm"
)

func TestGormOutboxNilGuards(t *testing.T) {
	ctx := context.Background()
	if err := gormoutbox.MigrateOutbox(ctx, nil); !errors.Is(err, queue.ErrNotInitialized) {
		t.Fatalf("MigrateOutbox nil db error = %v, want ErrNotInitialized", err)
	}

	outbox := gormoutbox.NewGormOutbox(nil, nil)
	if err := outbox.Save(ctx, queue.NewTask("demo", map[string]string{"ok": "true"})); !errors.Is(err, queue.ErrNotInitialized) {
		t.Fatalf("Save nil db error = %v, want ErrNotInitialized", err)
	}
	if err := outbox.Flush(ctx, 1); !errors.Is(err, queue.ErrNotInitialized) {
		t.Fatalf("Flush nil db/client error = %v, want ErrNotInitialized", err)
	}
}

func TestOutboxRecordTableName(t *testing.T) {
	if got := (gormoutbox.OutboxRecord{}).TableName(); got != "system_queue_outbox" {
		t.Fatalf("OutboxRecord.TableName() = %q, want system_queue_outbox", got)
	}
}

func TestGormOutboxImplementsCoreOutbox(t *testing.T) {
	var _ queue.Outbox = gormoutbox.NewGormOutbox(nil, nil)
	var _ queue.Outbox = gormoutbox.New(nil, nil)
}
