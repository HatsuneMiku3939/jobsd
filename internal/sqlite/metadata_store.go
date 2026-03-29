package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

var (
	ErrMetadataNotFound = errors.New("instance metadata not found")
	ErrMetadataCorrupt  = errors.New("instance metadata is corrupt")
)

type MetadataStore struct {
	db *sql.DB
}

func NewMetadataStore(db *sql.DB) *MetadataStore {
	return &MetadataStore{db: db}
}

func (s *MetadataStore) Upsert(ctx context.Context, meta domain.InstanceMetadata) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin metadata upsert transaction: %w", err)
	}

	updatedAt := time.Now().UTC().Format(time.RFC3339)
	values := map[string]string{
		"instance_name":  meta.InstanceName,
		"created_at":     meta.CreatedAt.UTC().Format(time.RFC3339),
		"scheduler_port": strconv.Itoa(meta.SchedulerPort),
	}

	for key, value := range values {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO instance_metadata(key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
    value = excluded.value,
    updated_at = excluded.updated_at
`, key, value, updatedAt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upsert metadata %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit metadata upsert: %w", err)
	}

	return nil
}

func (s *MetadataStore) Get(ctx context.Context) (domain.InstanceMetadata, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT key, value
FROM instance_metadata
WHERE key IN ('instance_name', 'created_at', 'scheduler_port')
`)
	if err != nil {
		return domain.InstanceMetadata{}, fmt.Errorf("query instance metadata: %w", err)
	}
	defer rows.Close()

	values := make(map[string]string, 3)
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return domain.InstanceMetadata{}, fmt.Errorf("scan instance metadata: %w", err)
		}
		values[key] = value
	}

	if err := rows.Err(); err != nil {
		return domain.InstanceMetadata{}, fmt.Errorf("iterate instance metadata: %w", err)
	}

	instanceName, ok := values["instance_name"]
	if !ok {
		return domain.InstanceMetadata{}, ErrMetadataNotFound
	}
	createdAtRaw, ok := values["created_at"]
	if !ok {
		return domain.InstanceMetadata{}, ErrMetadataNotFound
	}
	schedulerPortRaw, ok := values["scheduler_port"]
	if !ok {
		return domain.InstanceMetadata{}, ErrMetadataNotFound
	}

	createdAt, err := time.Parse(time.RFC3339, createdAtRaw)
	if err != nil {
		return domain.InstanceMetadata{}, fmt.Errorf("%w: created_at: %v", ErrMetadataCorrupt, err)
	}
	schedulerPort, err := strconv.Atoi(schedulerPortRaw)
	if err != nil {
		return domain.InstanceMetadata{}, fmt.Errorf("%w: scheduler_port: %v", ErrMetadataCorrupt, err)
	}

	return domain.InstanceMetadata{
		InstanceName:  instanceName,
		CreatedAt:     createdAt.UTC(),
		SchedulerPort: schedulerPort,
	}, nil
}
