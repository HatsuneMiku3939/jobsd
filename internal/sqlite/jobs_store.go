package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

var ErrJobNotFound = errors.New("job not found")

type JobStore struct {
	db *sql.DB
}

func NewJobStore(db *sql.DB) *JobStore {
	return &JobStore{db: db}
}

func (s *JobStore) Create(ctx context.Context, job domain.Job) (domain.Job, error) {
	if err := validateJob(job); err != nil {
		return domain.Job{}, err
	}

	result, err := s.db.ExecContext(ctx, `
INSERT INTO jobs (
    name,
    command,
    schedule_kind,
    schedule_expr,
    timezone,
    enabled,
    concurrency_policy,
    next_run_at,
    last_run_at,
    last_run_status,
    created_at,
    updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		job.Name,
		job.Command,
		string(job.ScheduleKind),
		job.ScheduleExpr,
		defaultTimezone(job.Timezone),
		boolToInt(job.Enabled),
		defaultConcurrencyPolicy(job.ConcurrencyPolicy),
		formatTimePtr(job.NextRunAt),
		formatTimePtr(job.LastRunAt),
		formatRunStatusPtr(job.LastRunStatus),
		formatTime(job.CreatedAt),
		formatTime(job.UpdatedAt),
	)
	if err != nil {
		return domain.Job{}, fmt.Errorf("insert job: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return domain.Job{}, fmt.Errorf("load inserted job id: %w", err)
	}

	created, err := s.GetByID(ctx, id)
	if err != nil {
		return domain.Job{}, fmt.Errorf("load inserted job: %w", err)
	}

	return created, nil
}

func (s *JobStore) GetByName(ctx context.Context, name string) (domain.Job, error) {
	row := s.db.QueryRowContext(ctx, jobSelectColumns+` WHERE name = ?`, name)

	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Job{}, ErrJobNotFound
		}
		return domain.Job{}, fmt.Errorf("get job by name %q: %w", name, err)
	}

	return job, nil
}

func (s *JobStore) GetByID(ctx context.Context, id int64) (domain.Job, error) {
	row := s.db.QueryRowContext(ctx, jobSelectColumns+` WHERE id = ?`, id)

	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Job{}, ErrJobNotFound
		}
		return domain.Job{}, fmt.Errorf("get job by id %d: %w", id, err)
	}

	return job, nil
}

func (s *JobStore) List(ctx context.Context) ([]domain.Job, error) {
	rows, err := s.db.QueryContext(ctx, jobSelectColumns+` ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs, err := scanJobs(rows)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}

	return jobs, nil
}

func (s *JobStore) Update(ctx context.Context, job domain.Job) (domain.Job, error) {
	if err := validateJob(job); err != nil {
		return domain.Job{}, err
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE jobs
SET
    name = ?,
    command = ?,
    schedule_kind = ?,
    schedule_expr = ?,
    timezone = ?,
    enabled = ?,
    concurrency_policy = ?,
    next_run_at = ?,
    last_run_at = ?,
    last_run_status = ?,
    updated_at = ?
WHERE id = ?
`,
		job.Name,
		job.Command,
		string(job.ScheduleKind),
		job.ScheduleExpr,
		defaultTimezone(job.Timezone),
		boolToInt(job.Enabled),
		defaultConcurrencyPolicy(job.ConcurrencyPolicy),
		formatTimePtr(job.NextRunAt),
		formatTimePtr(job.LastRunAt),
		formatRunStatusPtr(job.LastRunStatus),
		formatTime(job.UpdatedAt),
		job.ID,
	)
	if err != nil {
		return domain.Job{}, fmt.Errorf("update job %d: %w", job.ID, err)
	}

	if err := ensureRowsAffected(result, ErrJobNotFound); err != nil {
		return domain.Job{}, fmt.Errorf("update job %d: %w", job.ID, err)
	}

	updated, err := s.GetByID(ctx, job.ID)
	if err != nil {
		return domain.Job{}, fmt.Errorf("load updated job %d: %w", job.ID, err)
	}

	return updated, nil
}

func (s *JobStore) DeleteByName(ctx context.Context, name string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete job %q: %w", name, err)
	}

	if err := ensureRowsAffected(result, ErrJobNotFound); err != nil {
		return fmt.Errorf("delete job %q: %w", name, err)
	}

	return nil
}

func (s *JobStore) ListDue(ctx context.Context, now time.Time) ([]domain.Job, error) {
	rows, err := s.db.QueryContext(ctx, jobSelectColumns+`
 WHERE enabled = 1
   AND next_run_at IS NOT NULL
   AND next_run_at <= ?
 ORDER BY next_run_at ASC, id ASC
`, formatTime(now))
	if err != nil {
		return nil, fmt.Errorf("list due jobs: %w", err)
	}
	defer rows.Close()

	jobs, err := scanJobs(rows)
	if err != nil {
		return nil, fmt.Errorf("list due jobs: %w", err)
	}

	return jobs, nil
}

func (s *JobStore) UpdateNextRun(ctx context.Context, jobID int64, nextRunAt *time.Time, updatedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE jobs
SET
    next_run_at = ?,
    updated_at = ?
WHERE id = ?
`, formatTimePtr(nextRunAt), formatTime(updatedAt), jobID)
	if err != nil {
		return fmt.Errorf("update next run for job %d: %w", jobID, err)
	}

	if err := ensureRowsAffected(result, ErrJobNotFound); err != nil {
		return fmt.Errorf("update next run for job %d: %w", jobID, err)
	}

	return nil
}

func (s *JobStore) UpdateLastRunSummary(
	ctx context.Context,
	jobID int64,
	lastRunAt *time.Time,
	lastRunStatus *domain.RunStatus,
	updatedAt time.Time,
) error {
	if lastRunStatus != nil && !lastRunStatus.IsValid() {
		return fmt.Errorf("invalid last run status %q", *lastRunStatus)
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE jobs
SET
    last_run_at = ?,
    last_run_status = ?,
    updated_at = ?
WHERE id = ?
`, formatTimePtr(lastRunAt), formatRunStatusPtr(lastRunStatus), formatTime(updatedAt), jobID)
	if err != nil {
		return fmt.Errorf("update last run summary for job %d: %w", jobID, err)
	}

	if err := ensureRowsAffected(result, ErrJobNotFound); err != nil {
		return fmt.Errorf("update last run summary for job %d: %w", jobID, err)
	}

	return nil
}

const jobSelectColumns = `
SELECT
    id,
    name,
    command,
    schedule_kind,
    schedule_expr,
    timezone,
    enabled,
    concurrency_policy,
    next_run_at,
    last_run_at,
    last_run_status,
    created_at,
    updated_at
FROM jobs`

type jobScanner interface {
	Scan(dest ...any) error
}

func scanJobs(rows *sql.Rows) ([]domain.Job, error) {
	var jobs []domain.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}

	return jobs, nil
}

func scanJob(scanner jobScanner) (domain.Job, error) {
	var (
		job                  domain.Job
		scheduleKindRaw      string
		enabledRaw           int
		concurrencyPolicyRaw string
		nextRunAtRaw         sql.NullString
		lastRunAtRaw         sql.NullString
		lastRunStatusRaw     sql.NullString
		createdAtRaw         string
		updatedAtRaw         string
	)

	err := scanner.Scan(
		&job.ID,
		&job.Name,
		&job.Command,
		&scheduleKindRaw,
		&job.ScheduleExpr,
		&job.Timezone,
		&enabledRaw,
		&concurrencyPolicyRaw,
		&nextRunAtRaw,
		&lastRunAtRaw,
		&lastRunStatusRaw,
		&createdAtRaw,
		&updatedAtRaw,
	)
	if err != nil {
		return domain.Job{}, err
	}

	job.ScheduleKind = domain.ScheduleKind(scheduleKindRaw)
	if !job.ScheduleKind.IsValid() {
		return domain.Job{}, fmt.Errorf("invalid schedule kind %q", scheduleKindRaw)
	}

	job.ConcurrencyPolicy = domain.ConcurrencyPolicy(concurrencyPolicyRaw)
	if !job.ConcurrencyPolicy.IsValid() {
		return domain.Job{}, fmt.Errorf("invalid concurrency policy %q", concurrencyPolicyRaw)
	}

	enabled, err := intToBool(enabledRaw)
	if err != nil {
		return domain.Job{}, fmt.Errorf("invalid enabled flag for job %d: %w", job.ID, err)
	}
	job.Enabled = enabled

	job.NextRunAt, err = parseTimePtr(nextRunAtRaw)
	if err != nil {
		return domain.Job{}, fmt.Errorf("parse next_run_at for job %d: %w", job.ID, err)
	}

	job.LastRunAt, err = parseTimePtr(lastRunAtRaw)
	if err != nil {
		return domain.Job{}, fmt.Errorf("parse last_run_at for job %d: %w", job.ID, err)
	}

	job.LastRunStatus, err = parseRunStatusPtr(lastRunStatusRaw)
	if err != nil {
		return domain.Job{}, fmt.Errorf("parse last_run_status for job %d: %w", job.ID, err)
	}

	job.CreatedAt, err = parseTime(createdAtRaw)
	if err != nil {
		return domain.Job{}, fmt.Errorf("parse created_at for job %d: %w", job.ID, err)
	}

	job.UpdatedAt, err = parseTime(updatedAtRaw)
	if err != nil {
		return domain.Job{}, fmt.Errorf("parse updated_at for job %d: %w", job.ID, err)
	}

	return job, nil
}

func validateJob(job domain.Job) error {
	if !job.ScheduleKind.IsValid() {
		return fmt.Errorf("invalid schedule kind %q", job.ScheduleKind)
	}
	if policy := defaultConcurrencyPolicy(job.ConcurrencyPolicy); !domain.ConcurrencyPolicy(policy).IsValid() {
		return fmt.Errorf("invalid concurrency policy %q", job.ConcurrencyPolicy)
	}
	if job.LastRunStatus != nil && !job.LastRunStatus.IsValid() {
		return fmt.Errorf("invalid last run status %q", *job.LastRunStatus)
	}

	return nil
}

func ensureRowsAffected(result sql.Result, notFound error) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows: %w", err)
	}
	if rowsAffected == 0 {
		return notFound
	}

	return nil
}

func defaultTimezone(timezone string) string {
	if timezone == "" {
		return "Local"
	}

	return timezone
}

func defaultConcurrencyPolicy(policy domain.ConcurrencyPolicy) string {
	if policy == "" {
		return string(domain.ConcurrencyPolicyForbid)
	}

	return string(policy)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}

	return 0
}

func intToBool(value int) (bool, error) {
	switch value {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("expected 0 or 1, got %d", value)
	}
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func formatTimePtr(value *time.Time) any {
	if value == nil {
		return nil
	}

	return formatTime(*value)
}

func formatRunStatusPtr(value *domain.RunStatus) any {
	if value == nil {
		return nil
	}

	return string(*value)
}

func parseTime(raw string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}

	return parsed.UTC(), nil
}

func parseTimePtr(raw sql.NullString) (*time.Time, error) {
	if !raw.Valid {
		return nil, nil
	}

	parsed, err := parseTime(raw.String)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func parseRunStatusPtr(raw sql.NullString) (*domain.RunStatus, error) {
	if !raw.Valid {
		return nil, nil
	}

	status := domain.RunStatus(raw.String)
	if !status.IsValid() {
		return nil, fmt.Errorf("invalid run status %q", raw.String)
	}

	return &status, nil
}
