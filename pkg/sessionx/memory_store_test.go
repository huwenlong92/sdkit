package sessionx

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreSessionLifecycle(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	sess := &Session{
		ID:          "sid",
		SubjectID:   1001,
		SubjectType: "admin",
		Username:    "root",
		RoleID:      1,
		Permissions: []string{"system:read"},
		Extra:       map[string]any{"dept_id": int64(10)},
	}
	if err := store.Set(ctx, sess, time.Minute); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	got, ok := store.Get(ctx, "sid")
	if !ok {
		t.Fatal("session should exist")
	}
	if got.SubjectID != 1001 || got.SubjectType != "admin" {
		t.Fatalf("unexpected subject: id=%d type=%s", got.SubjectID, got.SubjectType)
	}
	if deptID, ok := got.GetExtraInt64("dept_id"); !ok || deptID != 10 {
		t.Fatalf("dept_id: want 10, got %d ok=%v", deptID, ok)
	}

	if err := store.Delete(ctx, "sid"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if _, ok := store.Get(ctx, "sid"); ok {
		t.Fatal("session should be deleted")
	}
}
