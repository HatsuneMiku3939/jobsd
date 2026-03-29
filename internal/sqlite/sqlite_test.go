package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestOpenAndMigrate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data", "jobs.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate() second call error = %v", err)
	}

	assertTableExists(t, db, "schema_migrations")
	assertTableExists(t, db, "jobs")
	assertTableExists(t, db, "job_runs")
	assertTableExists(t, db, "job_run_outputs")
	assertTableExists(t, db, "instance_metadata")

	assertIndexExists(t, db, "idx_jobs_name")
	assertIndexExists(t, db, "idx_jobs_enabled_next_run_at")
	assertIndexExists(t, db, "idx_job_runs_job_id_queued_at")
	assertIndexExists(t, db, "idx_job_runs_status_queued_at")
	assertIndexExists(t, db, "idx_job_runs_scheduled_for")

	assertPragmaValue(t, db, "foreign_keys", "1")
	assertPragmaValue(t, db, "journal_mode", "wal")
	assertPragmaValue(t, db, "synchronous", "1")
	assertPragmaValue(t, db, "busy_timeout", "5000")
}

func TestMetadataStore(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	store := NewMetadataStore(db)

	createdAt := time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC)
	meta := domain.InstanceMetadata{
		InstanceName:  "dev",
		CreatedAt:     createdAt,
		SchedulerPort: 8080,
	}

	if err := store.Upsert(ctx, meta); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	got, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != meta {
		t.Fatalf("Get() = %#v, want %#v", got, meta)
	}

	firstUpdatedAt := metadataUpdatedAt(t, db, "scheduler_port")

	updated := domain.InstanceMetadata{
		InstanceName:  "dev",
		CreatedAt:     createdAt,
		SchedulerPort: 9090,
	}
	time.Sleep(1100 * time.Millisecond)
	if err := store.Upsert(ctx, updated); err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}

	got, err = store.Get(ctx)
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	if got != updated {
		t.Fatalf("Get() after update = %#v, want %#v", got, updated)
	}

	secondUpdatedAt := metadataUpdatedAt(t, db, "scheduler_port")
	if !secondUpdatedAt.After(firstUpdatedAt) {
		t.Fatalf("updated_at did not move forward: first=%v second=%v", firstUpdatedAt, secondUpdatedAt)
	}
}

func TestMetadataStoreErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("missing metadata", func(t *testing.T) {
		db := openMigratedTestDB(t)
		store := NewMetadataStore(db)

		_, err := store.Get(ctx)
		if !errors.Is(err, ErrMetadataNotFound) {
			t.Fatalf("Get() error = %v, want %v", err, ErrMetadataNotFound)
		}
	})

	t.Run("corrupt scheduler port", func(t *testing.T) {
		db := openMigratedTestDB(t)
		store := NewMetadataStore(db)

		meta := domain.InstanceMetadata{
			InstanceName:  "dev",
			CreatedAt:     time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC),
			SchedulerPort: 8080,
		}
		if err := store.Upsert(ctx, meta); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE instance_metadata SET value = 'bad-port' WHERE key = 'scheduler_port'`); err != nil {
			t.Fatalf("ExecContext() error = %v", err)
		}

		_, err := store.Get(ctx)
		if !errors.Is(err, ErrMetadataCorrupt) {
			t.Fatalf("Get() error = %v, want %v", err, ErrMetadataCorrupt)
		}
	})
}

func openMigratedTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "jobs.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	return db
}

func assertTableExists(t *testing.T, db *sql.DB, name string) {
	t.Helper()

	var actual string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&actual); err != nil {
		t.Fatalf("table %q missing: %v", name, err)
	}
}

func assertIndexExists(t *testing.T, db *sql.DB, name string) {
	t.Helper()

	var actual string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&actual); err != nil {
		t.Fatalf("index %q missing: %v", name, err)
	}
}

func assertPragmaValue(t *testing.T, db *sql.DB, pragma string, want string) {
	t.Helper()

	var got string
	if err := db.QueryRow(`PRAGMA ` + pragma).Scan(&got); err != nil {
		t.Fatalf("PRAGMA %s error = %v", pragma, err)
	}
	if got != want {
		t.Fatalf("PRAGMA %s = %q, want %q", pragma, got, want)
	}
}

func metadataUpdatedAt(t *testing.T, db *sql.DB, key string) time.Time {
	t.Helper()

	var raw string
	if err := db.QueryRow(`SELECT updated_at FROM instance_metadata WHERE key = ?`, key).Scan(&raw); err != nil {
		t.Fatalf("select updated_at error = %v", err)
	}

	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("Parse(updated_at) error = %v", err)
	}

	return value
}
