package database

import (
	"path/filepath"
	"testing"
)

func TestOpenAndMigrateAppliesAllMigrations(t *testing.T) {
	t.Parallel()

	dsn := filepath.Join(t.TempDir(), "migration-check.db")
	db, err := OpenAndMigrate(dsn)
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	defer func() {
		_ = sqlDB.Close()
	}()
}
