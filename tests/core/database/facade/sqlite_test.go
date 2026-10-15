package facade_test

import (
	"context"
	"path/filepath"
	"testing"

	coredatabase "github.com/huwenlong92/sdkit/core/database"
	database "github.com/huwenlong92/sdkit/core/database/facade"
	"github.com/huwenlong92/sdkit/core/runtime"
)

func TestSQLiteCapabilityStartAndStop(t *testing.T) {
	t.Cleanup(coredatabase.Close)
	app := runtime.New()
	if err := app.Use(database.Use(database.WithConfig(database.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "runtime.db"), LogLevel: "silent"}))); err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	db := coredatabase.From(app)
	if db == nil || db.Gorm == nil || db.PGX != nil {
		t.Fatal("SQLite capability not bound correctly")
	}
	sqlDB, err := db.Gorm.DB()
	if err != nil {
		t.Fatal(err)
	}
	var got int
	if err := db.Gorm.Raw(`SELECT json_extract('{"ok":1}', '$.ok')`).Scan(&got).Error; err != nil || got != 1 {
		t.Fatalf("JSON: %d %v", got, err)
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Ping(); err == nil {
		t.Fatal("connection must be closed on Stop")
	}
	if coredatabase.Default != nil {
		t.Fatal("default database must be cleared")
	}
	if err := app.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}
