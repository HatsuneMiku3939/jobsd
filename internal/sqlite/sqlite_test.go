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
	assertTableExists(t, db, "run_hook_deliveries")
	assertTableExists(t, db, "instance_metadata")

	assertIndexExists(t, db, "idx_jobs_name")
	assertIndexExists(t, db, "idx_jobs_enabled_next_run_at")
	assertIndexExists(t, db, "idx_job_runs_job_id_queued_at")
	assertIndexExists(t, db, "idx_job_runs_status_queued_at")
	assertIndexExists(t, db, "idx_job_runs_scheduled_for")
	assertIndexExists(t, db, "idx_run_hook_deliveries_run_id_attempt")

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
		OnFinish: &domain.OnFinishConfig{
			Type: domain.OnFinishSinkTypeCommand,
			Command: &domain.CommandSinkConfig{
				Program: "echo",
			},
		},
	}

	if err := store.Upsert(ctx, meta); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	got, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	assertInstanceMetadataEqual(t, got, meta)

	firstUpdatedAt := metadataUpdatedAt(t, db, "scheduler_port")

	updated := domain.InstanceMetadata{
		InstanceName:  "dev",
		CreatedAt:     createdAt,
		SchedulerPort: 9090,
		OnFinish: &domain.OnFinishConfig{
			Type: domain.OnFinishSinkTypeHTTP,
			HTTP: &domain.HTTPSinkConfig{
				URL: "http://127.0.0.1:8080/hooks",
			},
		},
	}
	time.Sleep(1100 * time.Millisecond)
	if err := store.Upsert(ctx, updated); err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}

	got, err = store.Get(ctx)
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	assertInstanceMetadataEqual(t, got, updated)

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

	t.Run("corrupt on_finish config", func(t *testing.T) {
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
		if _, err := db.ExecContext(ctx, `INSERT INTO instance_metadata(key, value, updated_at) VALUES ('on_finish_json', 'bad-json', ?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
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

func assertInstanceMetadataEqual(t *testing.T, got domain.InstanceMetadata, want domain.InstanceMetadata) {
	t.Helper()

	if got.InstanceName != want.InstanceName {
		t.Fatalf("InstanceName = %q, want %q", got.InstanceName, want.InstanceName)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	if got.SchedulerPort != want.SchedulerPort {
		t.Fatalf("SchedulerPort = %d, want %d", got.SchedulerPort, want.SchedulerPort)
	}

	gotJSON, err := domain.MarshalOnFinishConfigJSON(got.OnFinish)
	if err != nil {
		t.Fatalf("MarshalOnFinishConfigJSON(got) error = %v", err)
	}
	wantJSON, err := domain.MarshalOnFinishConfigJSON(want.OnFinish)
	if err != nil {
		t.Fatalf("MarshalOnFinishConfigJSON(want) error = %v", err)
	}
	switch {
	case gotJSON == nil && wantJSON == nil:
		return
	case gotJSON == nil || wantJSON == nil:
		t.Fatalf("OnFinish = %v, want %v", got.OnFinish, want.OnFinish)
	case *gotJSON != *wantJSON:
		t.Fatalf("OnFinish = %s, want %s", *gotJSON, *wantJSON)
	}
}
