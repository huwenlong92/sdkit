package tests

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/core/database"
	"github.com/huwenlong92/sdkit/core/runtime"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
)

func TestSQLitePersistenceJSONAndTransactions(t *testing.T) {
	cfg := database.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "ledger.db"), LogLevel: "silent"}
	db, err := database.New(context.Background(), cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	if db.PGX != nil {
		t.Fatal("SQLite must not create PGX")
	}
	sqlDB, err := db.Gorm.DB()
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB.Stats().MaxOpenConnections != 1 {
		t.Fatal("SQLite should default to one connection")
	}
	if err := db.Gorm.Exec("CREATE TABLE sample (id INTEGER PRIMARY KEY, value TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Gorm.Exec("INSERT INTO sample(value) VALUES (?)", `{"bytes":123}`).Error; err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Gorm.Raw("SELECT json_extract(value, '$.bytes') FROM sample").Scan(&count).Error; err != nil || count != 123 {
		t.Fatalf("JSON: %d, %v", count, err)
	}
	sentinel := pgx.ErrNoRows
	err = db.Gorm.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO sample(value) VALUES (?)", `{}`).Error; err != nil {
			return err
		}
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("rollback error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := database.New(context.Background(), cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Gorm.Raw("SELECT count(*) FROM sample").Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("persistence: %d, %v", count, err)
	}
}

func TestDriverValidationAndPostgresDefault(t *testing.T) {
	for _, driver := range []string{"", "postgres"} {
		_, err := database.New(context.Background(), database.Config{Driver: driver, DSN: "definitely not a postgres DSN", LogLevel: "silent"}, "test")
		if err == nil || strings.Contains(err.Error(), "unsupported driver") {
			t.Fatalf("postgres path: %v", err)
		}
	}
	_, err := database.New(context.Background(), database.Config{Driver: "unknown", DSN: "ignored"}, "test")
	if err == nil || !strings.Contains(err.Error(), "unsupported driver") {
		t.Fatalf("unsupported driver: %v", err)
	}
	_, err = database.New(context.Background(), database.Config{Driver: "sqlite", DSN: "ignored", Schema: "public"}, "test")
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("sqlite schema config: %v", err)
	}
	app := runtime.New()
	if err := database.Bind(app, nil); err == nil {
		t.Fatal("nil binding must fail")
	}
}
