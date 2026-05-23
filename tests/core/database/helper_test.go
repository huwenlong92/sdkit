package tests

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/database"

	"gorm.io/gorm"
)

func TestHelpersReturnNilWhenDatabaseUnavailable(t *testing.T) {
	restoreDatabase(t)

	if got := database.Gorm(context.Background()); got != nil {
		t.Fatalf("Gorm() = %p, want nil", got)
	}
	if got := database.PGX(context.Background()); got != nil {
		t.Fatalf("PGX() = %p, want nil", got)
	}
}

func TestTransactionReturnsNotInitialized(t *testing.T) {
	restoreDatabase(t)

	err := database.Transaction(context.Background(), func(tx *gorm.DB) error {
		return nil
	})
	if !stderrors.Is(err, database.ErrNotInitialized) {
		t.Fatalf("Transaction() error = %v, want ErrNotInitialized", err)
	}
}

func restoreDatabase(t *testing.T) {
	t.Helper()
	prevDefault := database.Default
	prevDB := database.DB
	prevPGX := database.PGXPool
	database.Default = nil
	database.DB = nil
	database.PGXPool = nil
	t.Cleanup(func() {
		database.Default = prevDefault
		database.DB = prevDB
		database.PGXPool = prevPGX
	})
}
