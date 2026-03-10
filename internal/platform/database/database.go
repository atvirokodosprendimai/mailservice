package database

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/glebarez/sqlite"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// OpenAndMigrate opens a local SQLite database and runs migrations.
func OpenAndMigrate(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("gorm db: %w", err)
	}

	if err := migrate(sqlDB); err != nil {
		return nil, err
	}

	return db, nil
}

// OpenTurso opens a remote Turso database via libsql and wraps it in GORM.
// The url should be in the format: libsql://[DATABASE].turso.io?authToken=[TOKEN]
func OpenTurso(url string, authToken string) (*gorm.DB, error) {
	dsn := url
	if authToken != "" {
		dsn += "?authToken=" + authToken
	}

	sqlDB, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open turso: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("turso ping: %w", err)
	}

	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("gorm turso: %w", err)
	}

	if err := migrate(sqlDB); err != nil {
		return nil, err
	}

	return db, nil
}

func migrate(sqlDB *sql.DB) error {
	goose.SetBaseFS(migrationFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
