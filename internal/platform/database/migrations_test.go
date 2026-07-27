package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
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

// Versions bracketing the 20260727120000 rename migration under test:
// preRenamePaymentSessionVersion is the last migration applied before it,
// and renamePaymentSessionVersion is its own goose version.
const (
	preRenamePaymentSessionVersion = 20260524070000
	renamePaymentSessionVersion    = 20260727120000
)

// openPreMigratedDB opens a fresh sqlite DB and applies all migrations up to
// (and including) upToVersion, without going through the package-level
// migrate() helper, so tests can seed rows between specific migration steps.
//
// No t.Parallel() in callers — goose.SetBaseFS/SetDialect are global state.
func openPreMigratedDB(t *testing.T, upToVersion int64) *sql.DB {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "rename-payment-session.db")
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open failed: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	goose.SetBaseFS(migrationFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("goose SetDialect failed: %v", err)
	}
	if err := goose.UpTo(sqlDB, "migrations", upToVersion); err != nil {
		t.Fatalf("goose UpTo(%d) failed: %v", upToVersion, err)
	}
	return sqlDB
}

func seedPreRenameMailbox(t *testing.T, sqlDB *sql.DB, id, sessionID string) {
	t.Helper()

	_, err := sqlDB.Exec(`INSERT INTO mailboxes (
		id, owner_email, billing_email, imap_host, imap_port, imap_username,
		imap_password, access_token, stripe_session_id, payment_url, status, account_id
	) VALUES (?, ?, '', 'imap.example.com', 143, ?, 'secret', ?, ?, '', 'pending_payment', '')`,
		id, id+"@example.com", id, id+"-token", sessionID)
	if err != nil {
		t.Fatalf("seed mailbox %s failed: %v", id, err)
	}
}

func TestRenamePaymentSessionMigrationBackfillsProviderByPrefix(t *testing.T) {
	sqlDB := openPreMigratedDB(t, preRenamePaymentSessionVersion)

	seedPreRenameMailbox(t, sqlDB, "mbx-stripe", "cs_test_stripe123")
	seedPreRenameMailbox(t, sqlDB, "mbx-polar", "pol_test_polar456")
	seedPreRenameMailbox(t, sqlDB, "mbx-empty", "")

	if err := goose.UpTo(sqlDB, "migrations", renamePaymentSessionVersion); err != nil {
		t.Fatalf("goose UpTo(rename) failed: %v", err)
	}

	rows, err := sqlDB.Query(`SELECT id, payment_session_id, payment_provider FROM mailboxes ORDER BY id`)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	got := map[string]struct{ sessionID, provider string }{}
	for rows.Next() {
		var id, sessionID, provider string
		if err := rows.Scan(&id, &sessionID, &provider); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		got[id] = struct{ sessionID, provider string }{sessionID, provider}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration failed: %v", err)
	}

	if got["mbx-stripe"].sessionID != "cs_test_stripe123" {
		t.Fatalf("expected payment_session_id to survive rename, got %q", got["mbx-stripe"].sessionID)
	}
	if got["mbx-stripe"].provider != "stripe" {
		t.Fatalf("expected cs_-prefixed row to backfill to 'stripe', got %q", got["mbx-stripe"].provider)
	}
	if got["mbx-polar"].provider != "polar" {
		t.Fatalf("expected non-cs_-prefixed non-empty row to backfill to 'polar', got %q", got["mbx-polar"].provider)
	}
	if got["mbx-empty"].provider != "paddle" {
		t.Fatalf("expected empty session ID row to keep default 'paddle', got %q", got["mbx-empty"].provider)
	}
}

func TestRenamePaymentSessionMigrationUniqueIndexRejectsDuplicate(t *testing.T) {
	sqlDB := openPreMigratedDB(t, preRenamePaymentSessionVersion)

	seedPreRenameMailbox(t, sqlDB, "mbx-first", "cs_test_dup")

	if err := goose.UpTo(sqlDB, "migrations", renamePaymentSessionVersion); err != nil {
		t.Fatalf("goose UpTo(rename) failed: %v", err)
	}

	_, err := sqlDB.Exec(`INSERT INTO mailboxes (
		id, owner_email, billing_email, imap_host, imap_port, imap_username,
		imap_password, access_token, payment_session_id, payment_url, status, account_id, payment_provider
	) VALUES ('mbx-second', 'second@example.com', '', 'imap.example.com', 143, 'mbx-second', 'secret', 'second-token', 'cs_test_dup', '', 'pending_payment', '', 'stripe')`)
	if err == nil {
		t.Fatal("expected duplicate payment_session_id insert to violate the partial unique index")
	}
}

func TestRenamePaymentSessionMigrationDownRestoresColumnAndDropsAdded(t *testing.T) {
	sqlDB := openPreMigratedDB(t, renamePaymentSessionVersion)

	if err := goose.DownTo(sqlDB, "migrations", preRenamePaymentSessionVersion); err != nil {
		t.Fatalf("goose DownTo failed: %v", err)
	}

	rows, err := sqlDB.Query(`PRAGMA table_info(mailboxes)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info failed: %v", err)
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue any
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan pragma row failed: %v", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration failed: %v", err)
	}

	if !columns["stripe_session_id"] {
		t.Fatal("expected stripe_session_id column to be restored after down migration")
	}
	for _, dropped := range []string{"payment_session_id", "payment_provider", "subscription_id", "last_payment_event_at", "last_payment_event_id"} {
		if columns[dropped] {
			t.Fatalf("expected column %q to be dropped by down migration", dropped)
		}
	}
}
