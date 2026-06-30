package tests

import (
	"context"
	"testing"

	"github.com/huwenlong92/sdkit/core/reporter"
)

func TestProgressSnapshotSinkUpdatesStepsAndKeepsTerminal(t *testing.T) {
	ctx := context.Background()
	store := reporter.NewMemoryProgressStore()
	r := reporter.New(reporter.NewProgressSnapshotSink(store, reporter.WithProgressRecentLimit(2)))

	if err := r.Progress(ctx, reporter.Event{
		OperationID: "prepare:1",
		Domain:      "teval",
		Step:        "download",
		ProgressKey: "download_data",
		Percent:     10,
		Status:      "running",
		Message:     "start download",
		Subject:     map[string]any{"dataset_id": int64(1)},
	}); err != nil {
		t.Fatalf("progress 1: %v", err)
	}
	if err := r.Progress(ctx, reporter.Event{
		OperationID: "prepare:1",
		Step:        "download",
		ProgressKey: "download_data",
		Percent:     40,
		Status:      "running",
		Message:     "download 40%",
	}); err != nil {
		t.Fatalf("progress 2: %v", err)
	}
	if err := r.Progress(ctx, reporter.Event{
		OperationID: "prepare:1",
		Step:        "parse",
		ProgressKey: "parse_data",
		Percent:     60,
		Status:      "running",
		Message:     "parse csv",
	}); err != nil {
		t.Fatalf("progress 3: %v", err)
	}
	if err := r.Done(ctx, reporter.Event{
		OperationID: "prepare:1",
		Step:        "success",
		Percent:     100,
		Status:      "success",
		Message:     "done",
	}); err != nil {
		t.Fatalf("done: %v", err)
	}

	snapshot, ok, err := store.Load(ctx, "prepare:1")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("snapshot missing")
	}
	if !snapshot.Terminal || snapshot.Status != "success" || snapshot.Percent != 100 {
		t.Fatalf("terminal snapshot = %#v", snapshot)
	}
	if len(snapshot.Steps) != 2 {
		t.Fatalf("steps = %#v, want download and parse", snapshot.Steps)
	}
	if snapshot.Steps[0].Key != "download_data" || snapshot.Steps[0].Percent != 40 {
		t.Fatalf("download step = %#v, want updated percent 40", snapshot.Steps[0])
	}
	if len(snapshot.Recent) != 2 {
		t.Fatalf("recent events = %d, want capped at 2", len(snapshot.Recent))
	}
	if snapshot.Subject["dataset_id"] != int64(1) {
		t.Fatalf("subject = %#v, want dataset_id retained", snapshot.Subject)
	}
}

func TestMemoryProgressStoreExpiresSnapshots(t *testing.T) {
	ctx := context.Background()
	store := reporter.NewMemoryProgressStore()
	if err := store.Save(ctx, "op-1", reporter.ProgressSnapshot{OperationID: "op-1"}, -1); err != nil {
		t.Fatalf("save: %v", err)
	}
	_, ok, err := store.Load(ctx, "op-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !ok {
		t.Fatalf("negative ttl should behave like no expiration")
	}
}

func TestProgressCleanupSinkDeletesOnlyTerminalSnapshots(t *testing.T) {
	ctx := context.Background()
	store := reporter.NewMemoryProgressStore()
	r := reporter.NewWithOptions(
		reporter.WithSink(reporter.NewProgressSnapshotSink(store)),
		reporter.WithSink(reporter.NewProgressCleanupSink(store)),
	)

	if err := r.Progress(ctx, reporter.Event{
		OperationID: "run:1",
		Step:        "build_input",
		Status:      "running",
		Percent:     30,
	}); err != nil {
		t.Fatalf("progress: %v", err)
	}
	if _, ok, err := store.Load(ctx, "run:1"); err != nil || !ok {
		t.Fatalf("snapshot after progress ok=%v err=%v, want present", ok, err)
	}

	if err := r.Done(ctx, reporter.Event{
		OperationID: "run:1",
		Step:        "completed",
		Status:      "success",
		Percent:     100,
	}); err != nil {
		t.Fatalf("done: %v", err)
	}
	if _, ok, err := store.Load(ctx, "run:1"); err != nil || ok {
		t.Fatalf("snapshot after done ok=%v err=%v, want deleted", ok, err)
	}
}
