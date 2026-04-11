package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

type RunHookDeliveryStore struct {
	db *sql.DB
}

func NewRunHookDeliveryStore(db *sql.DB) *RunHookDeliveryStore {
	return &RunHookDeliveryStore{db: db}
}

func (s *RunHookDeliveryStore) Create(ctx context.Context, delivery domain.RunHookDelivery) (domain.RunHookDelivery, error) {
	if delivery.RunID <= 0 {
		return domain.RunHookDelivery{}, fmt.Errorf("run_id must be set")
	}
	if delivery.Event == "" {
		return domain.RunHookDelivery{}, fmt.Errorf("event must be set")
	}
	if !delivery.SinkType.IsValid() {
		return domain.RunHookDelivery{}, fmt.Errorf("invalid sink type %q", delivery.SinkType)
	}
	if delivery.Attempt <= 0 {
		return domain.RunHookDelivery{}, fmt.Errorf("attempt must be >= 1")
	}
	if !delivery.Status.IsValid() {
		return domain.RunHookDelivery{}, fmt.Errorf("invalid delivery status %q", delivery.Status)
	}
	if delivery.StartedAt.IsZero() || delivery.FinishedAt.IsZero() {
		return domain.RunHookDelivery{}, fmt.Errorf("started_at and finished_at must be set")
	}

	result, err := s.db.ExecContext(ctx, `
INSERT INTO run_hook_deliveries (
    run_id,
    event,
    sink_type,
    attempt,
    status,
    http_status_code,
    error_message,
    started_at,
    finished_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		delivery.RunID,
		delivery.Event,
		string(delivery.SinkType),
		delivery.Attempt,
		string(delivery.Status),
		intPtrValue(delivery.HTTPStatusCode),
		stringPtrValue(delivery.ErrorMessage),
		formatTime(delivery.StartedAt),
		formatTime(delivery.FinishedAt),
	)
	if err != nil {
		return domain.RunHookDelivery{}, fmt.Errorf("insert run hook delivery: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return domain.RunHookDelivery{}, fmt.Errorf("load inserted hook delivery id: %w", err)
	}

	delivery.ID = id
	return delivery, nil
}

func (s *RunHookDeliveryStore) ListByRunID(ctx context.Context, runID int64) ([]domain.RunHookDelivery, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
    id,
    run_id,
    event,
    sink_type,
    attempt,
    status,
    http_status_code,
    error_message,
    started_at,
    finished_at
FROM run_hook_deliveries
WHERE run_id = ?
ORDER BY attempt ASC, id ASC
`, runID)
	if err != nil {
		return nil, fmt.Errorf("list run hook deliveries for run %d: %w", runID, err)
	}
	defer rows.Close()

	deliveries := make([]domain.RunHookDelivery, 0)
	for rows.Next() {
		delivery, err := scanRunHookDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run hook deliveries for run %d: %w", runID, err)
	}

	return deliveries, nil
}

func scanRunHookDelivery(scanner interface{ Scan(dest ...any) error }) (domain.RunHookDelivery, error) {
	var (
		delivery          domain.RunHookDelivery
		sinkTypeRaw       string
		statusRaw         string
		httpStatusCodeRaw sql.NullInt64
		errorMessageRaw   sql.NullString
		startedAtRaw      string
		finishedAtRaw     string
	)

	if err := scanner.Scan(
		&delivery.ID,
		&delivery.RunID,
		&delivery.Event,
		&sinkTypeRaw,
		&delivery.Attempt,
		&statusRaw,
		&httpStatusCodeRaw,
		&errorMessageRaw,
		&startedAtRaw,
		&finishedAtRaw,
	); err != nil {
		return domain.RunHookDelivery{}, err
	}

	delivery.SinkType = domain.OnFinishSinkType(sinkTypeRaw)
	if !delivery.SinkType.IsValid() {
		return domain.RunHookDelivery{}, fmt.Errorf("invalid sink type %q", sinkTypeRaw)
	}

	delivery.Status = domain.HookDeliveryStatus(statusRaw)
	if !delivery.Status.IsValid() {
		return domain.RunHookDelivery{}, fmt.Errorf("invalid delivery status %q", statusRaw)
	}

	if httpStatusCodeRaw.Valid {
		value := int(httpStatusCodeRaw.Int64)
		delivery.HTTPStatusCode = &value
	}
	if errorMessageRaw.Valid {
		value := errorMessageRaw.String
		delivery.ErrorMessage = &value
	}

	var err error
	delivery.StartedAt, err = parseTime(startedAtRaw)
	if err != nil {
		return domain.RunHookDelivery{}, fmt.Errorf("parse started_at: %w", err)
	}
	delivery.FinishedAt, err = parseTime(finishedAtRaw)
	if err != nil {
		return domain.RunHookDelivery{}, fmt.Errorf("parse finished_at: %w", err)
	}

	return delivery, nil
}
