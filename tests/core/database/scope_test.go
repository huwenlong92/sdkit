package tests

import (
	"strings"
	"testing"

	"github.com/huwenlong92/sdkit/core/database"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPaginateScope(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=localhost user=test dbname=test sslmode=disable"), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open dry run gorm: %v", err)
	}

	result := db.Table("items").Scopes(database.Paginate(3, 25)).Find(&[]map[string]any{})
	sql := result.Statement.SQL.String()

	if !strings.Contains(sql, "LIMIT $1") || !strings.Contains(sql, "OFFSET $2") {
		t.Fatalf("Paginate() SQL = %q, want limit and offset", sql)
	}
	if got, want := result.Statement.Vars, []any{25, 50}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Paginate() vars = %#v, want %#v", got, want)
	}
}

func TestPaginateScopeUsesPageDefaults(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=localhost user=test dbname=test sslmode=disable"), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open dry run gorm: %v", err)
	}

	result := db.Table("items").Scopes(database.Paginate(0, 0)).Find(&[]map[string]any{})
	if got, want := result.Statement.Vars, []any{20}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("Paginate() default vars = %#v, want %#v", got, want)
	}
}
