package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

var ErrRunNotFound = errors.New("run not found")

type RunStore struct {
	db *sql.DB
}

type FinishRunParams struct {
	RunID        int64
	Status       domain.RunStatus
	FinishedAt   time.Time
	ExitCode     *int
	ErrorMessage *string
	Output       *domain.RunOutput
}

type ListRunsFilter struct {
	JobName string
	Status  *domain.RunStatus
	Limit   int
}

func NewRunStore(db *sql.DB) *RunStore {
	return &RunStore{db: db}
}

func (s *RunStore) EnqueueManual(ctx context.Context, jobID int64, queuedAt time.Time) (domain.Run, error) {
	result, err := s.db.ExecContext(ctx, `
INSERT INTO job_runs (
    job_id,
    trigger_type,
    status,
    scheduled_for,
    queued_at
)
VALUES (?, ?, ?, NULL, ?)
`,
		jobID,
		string(domain.RunTriggerTypeManual),
		string(domain.RunStatusPending),
		formatTime(queuedAt),
	)
	if err != nil {
		return domain.Run{}, fmt.Errorf("enqueue manual run for job %d: %w", jobID, err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return domain.Run{}, fmt.Errorf("load inserted manual run id: %w", err)
	}

	run, err := s.getRun(ctx, s.db, id, false)
	if err != nil {
		return domain.Run{}, fmt.Errorf("load inserted manual run %d: %w", id, err)
	}

	return run, nil
}

func (s *RunStore) EnqueueScheduled(
	ctx context.Context,
	jobID int64,
	scheduledFor time.Time,
	queuedAt time.Time,
) (domain.Run, error) {
	result, err := s.db.ExecContext(ctx, `
INSERT INTO job_runs (
    job_id,
    trigger_type,
    status,
    scheduled_for,
    queued_at
)
VALUES (?, ?, ?, ?, ?)
`,
		jobID,
		string(domain.RunTriggerTypeSchedule),
		string(domain.RunStatusPending),
		formatTime(scheduledFor),
		formatTime(queuedAt),
	)
	if err != nil {
		return domain.Run{}, fmt.Errorf("enqueue scheduled run for job %d: %w", jobID, err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return domain.Run{}, fmt.Errorf("load inserted scheduled run id: %w", err)
	}

	run, err := s.getRun(ctx, s.db, id, false)
	if err != nil {
		return domain.Run{}, fmt.Errorf("load inserted scheduled run %d: %w", id, err)
	}

	return run, nil
}

func (s *RunStore) ListPending(ctx context.Context, limit int) ([]domain.Run, error) {
	if limit <= 0 {
		return []domain.Run{}, nil
	}

	rows, err := s.db.QueryContext(ctx, runBaseSelect+`
 WHERE jr.status = ?
   AND jr.runner_id IS NULL
 ORDER BY jr.queued_at ASC, jr.id ASC
 LIMIT ?
`, string(domain.RunStatusPending), limit)
	if err != nil {
		return nil, fmt.Errorf("list pending runs: %w", err)
	}
	defer rows.Close()

	runs, err := scanRuns(rows)
	if err != nil {
		return nil, fmt.Errorf("list pending runs: %w", err)
	}

	return runs, nil
}

func (s *RunStore) ClaimPending(ctx context.Context, runnerID string, limit int) ([]domain.Run, error) {
	if limit <= 0 {
		return []domain.Run{}, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim pending transaction: %w", err)
	}

	rows, err := tx.QueryContext(ctx, runBaseSelect+`
 WHERE jr.status = ?
   AND jr.runner_id IS NULL
 ORDER BY jr.queued_at ASC, jr.id ASC
 LIMIT ?
`, string(domain.RunStatusPending), limit)
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("query pending runs: %w", err)
	}

	candidates, err := scanRuns(rows)
	rows.Close()
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("scan pending runs: %w", err)
	}

	claimed := make([]domain.Run, 0, len(candidates))
	for _, run := range candidates {
		result, err := tx.ExecContext(ctx, `
UPDATE job_runs
SET runner_id = ?
WHERE id = ?
  AND status = ?
  AND runner_id IS NULL
`, runnerID, run.ID, string(domain.RunStatusPending))
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("claim run %d: %w", run.ID, err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("read claim result for run %d: %w", run.ID, err)
		}
		if rowsAffected == 0 {
			continue
		}

		run.RunnerID = stringPtr(runnerID)
		claimed = append(claimed, run)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim pending transaction: %w", err)
	}

	return claimed, nil
}

func (s *RunStore) TryClaimPending(ctx context.Context, runID int64, runnerID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
UPDATE job_runs
SET runner_id = ?
WHERE id = ?
  AND status = ?
  AND runner_id IS NULL
`, runnerID, runID, string(domain.RunStatusPending))
	if err != nil {
		return false, fmt.Errorf("try claim run %d: %w", runID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("try claim run %d: read affected rows: %w", runID, err)
	}

	return rowsAffected > 0, nil
}

func (s *RunStore) MarkRunning(ctx context.Context, runID int64, startedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE job_runs
SET
    status = ?,
    started_at = ?
WHERE id = ?
  AND status = ?
`, string(domain.RunStatusRunning), formatTime(startedAt), runID, string(domain.RunStatusPending))
	if err != nil {
		return fmt.Errorf("mark run %d running: %w", runID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark run %d running: read affected rows: %w", runID, err)
	}
	if rowsAffected > 0 {
		return nil
	}

	exists, err := s.runExists(ctx, runID)
	if err != nil {
		return fmt.Errorf("mark run %d running: %w", runID, err)
	}
	if !exists {
		return ErrRunNotFound
	}

	return fmt.Errorf("mark run %d running: run is not pending", runID)
}

func (s *RunStore) ListUnfinishedByJob(ctx context.Context, jobID int64) ([]domain.Run, error) {
	rows, err := s.db.QueryContext(ctx, runBaseSelect+`
 WHERE jr.job_id = ?
   AND jr.status IN (?, ?)
 ORDER BY jr.queued_at ASC, jr.id ASC
`, jobID, string(domain.RunStatusPending), string(domain.RunStatusRunning))
	if err != nil {
		return nil, fmt.Errorf("list unfinished runs for job %d: %w", jobID, err)
	}
	defer rows.Close()

	runs, err := scanRuns(rows)
	if err != nil {
		return nil, fmt.Errorf("list unfinished runs for job %d: %w", jobID, err)
	}

	return runs, nil
}

func (s *RunStore) MarkFinished(ctx context.Context, params FinishRunParams) error {
	if !params.Status.IsValid() {
		return fmt.Errorf("invalid run status %q", params.Status)
	}
	if params.Status == domain.RunStatusPending || params.Status == domain.RunStatusRunning {
		return fmt.Errorf("run %d status %q is not terminal", params.RunID, params.Status)
	}
	if params.Output != nil {
		if params.Output.UpdatedAt.IsZero() {
			return fmt.Errorf("run %d output updated_at must be set", params.RunID)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mark finished transaction: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
UPDATE job_runs
SET
    status = ?,
    finished_at = ?,
    exit_code = ?,
    error_message = ?,
    runner_id = NULL
WHERE id = ?
`,
		string(params.Status),
		formatTime(params.FinishedAt),
		intPtrValue(params.ExitCode),
		stringPtrValue(params.ErrorMessage),
		params.RunID,
	)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("update run %d final state: %w", params.RunID, err)
	}

	if err := ensureRowsAffected(result, ErrRunNotFound); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("update run %d final state: %w", params.RunID, err)
	}

	if params.Output != nil {
		_, err := tx.ExecContext(ctx, `
INSERT INTO job_run_outputs (
    run_id,
    stdout_text,
    stderr_text,
    stdout_truncated,
    stderr_truncated,
    updated_at
)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id) DO UPDATE SET
    stdout_text = excluded.stdout_text,
    stderr_text = excluded.stderr_text,
    stdout_truncated = excluded.stdout_truncated,
    stderr_truncated = excluded.stderr_truncated,
    updated_at = excluded.updated_at
`,
			params.RunID,
			params.Output.Stdout,
			params.Output.Stderr,
			boolToInt(params.Output.StdoutTruncated),
			boolToInt(params.Output.StderrTruncated),
			formatTime(params.Output.UpdatedAt),
		)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upsert output for run %d: %w", params.RunID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mark finished transaction: %w", err)
	}

	return nil
}

func (s *RunStore) CancelPendingByJob(ctx context.Context, jobID int64, canceledAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE job_runs
SET
    status = ?,
    finished_at = ?,
    runner_id = NULL
WHERE job_id = ?
  AND status = ?
`,
		string(domain.RunStatusCanceled),
		formatTime(canceledAt),
		jobID,
		string(domain.RunStatusPending),
	)
	if err != nil {
		return fmt.Errorf("cancel pending runs for job %d: %w", jobID, err)
	}

	return nil
}

func (s *RunStore) List(ctx context.Context, filter ListRunsFilter) ([]domain.Run, error) {
	if filter.Status != nil && !filter.Status.IsValid() {
		return nil, fmt.Errorf("invalid run status filter %q", *filter.Status)
	}

	var (
		builder strings.Builder
		args    []any
	)

	builder.WriteString(runBaseSelect)
	builder.WriteString("\n WHERE 1 = 1")

	if filter.JobName != "" {
		builder.WriteString("\n   AND j.name = ?")
		args = append(args, filter.JobName)
	}

	if filter.Status != nil {
		builder.WriteString("\n   AND jr.status = ?")
		args = append(args, string(*filter.Status))
	}

	builder.WriteString("\n ORDER BY jr.queued_at DESC, jr.id DESC")
	if filter.Limit > 0 {
		builder.WriteString("\n LIMIT ?")
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	runs, err := scanRuns(rows)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	return runs, nil
}

func (s *RunStore) Get(ctx context.Context, runID int64) (domain.Run, error) {
	run, err := s.getRun(ctx, s.db, runID, true)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Run{}, ErrRunNotFound
		}
		return domain.Run{}, fmt.Errorf("get run %d: %w", runID, err)
	}

	return run, nil
}

const runBaseSelect = `
SELECT
    jr.id,
    jr.job_id,
    j.name,
    jr.trigger_type,
    jr.status,
    jr.scheduled_for,
    jr.queued_at,
    jr.started_at,
    jr.finished_at,
    jr.exit_code,
    jr.error_message,
    jr.runner_id
FROM job_runs jr
JOIN jobs j ON j.id = jr.job_id`

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *RunStore) getRun(ctx context.Context, querier queryRower, runID int64, includeOutput bool) (domain.Run, error) {
	if !includeOutput {
		row := querier.QueryRowContext(ctx, runBaseSelect+`
 WHERE jr.id = ?`, runID)
		return scanRun(row)
	}

	row := querier.QueryRowContext(ctx, `
SELECT
    jr.id,
    jr.job_id,
    j.name,
    jr.trigger_type,
    jr.status,
    jr.scheduled_for,
    jr.queued_at,
    jr.started_at,
    jr.finished_at,
    jr.exit_code,
    jr.error_message,
    jr.runner_id,
    jro.stdout_text,
    jro.stderr_text,
    jro.stdout_truncated,
    jro.stderr_truncated,
    jro.updated_at
FROM job_runs jr
JOIN jobs j ON j.id = jr.job_id
LEFT JOIN job_run_outputs jro ON jro.run_id = jr.id
WHERE jr.id = ?
`, runID)
	return scanRunWithOutput(row)
}

func (s *RunStore) runExists(ctx context.Context, runID int64) (bool, error) {
	var value int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM job_runs WHERE id = ?`, runID).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query run %d existence: %w", runID, err)
	}

	return true, nil
}

type runScanner interface {
	Scan(dest ...any) error
}

func scanRuns(rows *sql.Rows) ([]domain.Run, error) {
	var runs []domain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}

	return runs, nil
}

func scanRun(scanner runScanner) (domain.Run, error) {
	var (
		id              int64
		jobID           int64
		jobName         string
		triggerTypeRaw  string
		statusRaw       string
		scheduledForRaw sql.NullString
		queuedAtRaw     string
		startedAtRaw    sql.NullString
		finishedAtRaw   sql.NullString
		exitCodeRaw     sql.NullInt64
		errorMessageRaw sql.NullString
		runnerIDRaw     sql.NullString
	)

	err := scanner.Scan(
		&id,
		&jobID,
		&jobName,
		&triggerTypeRaw,
		&statusRaw,
		&scheduledForRaw,
		&queuedAtRaw,
		&startedAtRaw,
		&finishedAtRaw,
		&exitCodeRaw,
		&errorMessageRaw,
		&runnerIDRaw,
	)
	if err != nil {
		return domain.Run{}, err
	}

	return hydrateRunBase(
		id,
		jobID,
		jobName,
		triggerTypeRaw,
		statusRaw,
		scheduledForRaw,
		queuedAtRaw,
		startedAtRaw,
		finishedAtRaw,
		exitCodeRaw,
		errorMessageRaw,
		runnerIDRaw,
	)
}

func scanRunWithOutput(scanner runScanner) (domain.Run, error) {
	var (
		run                domain.Run
		triggerTypeRaw     string
		statusRaw          string
		scheduledForRaw    sql.NullString
		queuedAtRaw        string
		startedAtRaw       sql.NullString
		finishedAtRaw      sql.NullString
		exitCodeRaw        sql.NullInt64
		errorMessageRaw    sql.NullString
		runnerIDRaw        sql.NullString
		stdoutRaw          sql.NullString
		stderrRaw          sql.NullString
		stdoutTruncatedRaw sql.NullInt64
		stderrTruncatedRaw sql.NullInt64
		outputUpdatedAtRaw sql.NullString
	)

	err := scanner.Scan(
		&run.ID,
		&run.JobID,
		&run.JobName,
		&triggerTypeRaw,
		&statusRaw,
		&scheduledForRaw,
		&queuedAtRaw,
		&startedAtRaw,
		&finishedAtRaw,
		&exitCodeRaw,
		&errorMessageRaw,
		&runnerIDRaw,
		&stdoutRaw,
		&stderrRaw,
		&stdoutTruncatedRaw,
		&stderrTruncatedRaw,
		&outputUpdatedAtRaw,
	)
	if err != nil {
		return domain.Run{}, err
	}

	baseRun, err := hydrateRunBase(
		run.ID,
		run.JobID,
		run.JobName,
		triggerTypeRaw,
		statusRaw,
		scheduledForRaw,
		queuedAtRaw,
		startedAtRaw,
		finishedAtRaw,
		exitCodeRaw,
		errorMessageRaw,
		runnerIDRaw,
	)
	if err != nil {
		return domain.Run{}, err
	}
	run = baseRun

	if outputUpdatedAtRaw.Valid {
		stdoutTruncated, err := nullIntToBool(stdoutTruncatedRaw, "stdout_truncated")
		if err != nil {
			return domain.Run{}, fmt.Errorf("parse output for run %d: %w", run.ID, err)
		}
		stderrTruncated, err := nullIntToBool(stderrTruncatedRaw, "stderr_truncated")
		if err != nil {
			return domain.Run{}, fmt.Errorf("parse output for run %d: %w", run.ID, err)
		}
		updatedAt, err := parseTime(outputUpdatedAtRaw.String)
		if err != nil {
			return domain.Run{}, fmt.Errorf("parse output updated_at for run %d: %w", run.ID, err)
		}

		run.Output = &domain.RunOutput{
			Stdout:          stdoutRaw.String,
			Stderr:          stderrRaw.String,
			StdoutTruncated: stdoutTruncated,
			StderrTruncated: stderrTruncated,
			UpdatedAt:       updatedAt,
		}
	}

	return run, nil
}

func hydrateRunBase(
	id int64,
	jobID int64,
	jobName string,
	triggerTypeRaw string,
	statusRaw string,
	scheduledForRaw sql.NullString,
	queuedAtRaw string,
	startedAtRaw sql.NullString,
	finishedAtRaw sql.NullString,
	exitCodeRaw sql.NullInt64,
	errorMessageRaw sql.NullString,
	runnerIDRaw sql.NullString,
) (domain.Run, error) {
	run := domain.Run{
		ID:      id,
		JobID:   jobID,
		JobName: jobName,
	}

	triggerType := domain.RunTriggerType(triggerTypeRaw)
	if !triggerType.IsValid() {
		return domain.Run{}, fmt.Errorf("invalid run trigger type %q", triggerTypeRaw)
	}
	run.TriggerType = triggerType

	status := domain.RunStatus(statusRaw)
	if !status.IsValid() {
		return domain.Run{}, fmt.Errorf("invalid run status %q", statusRaw)
	}
	run.Status = status

	var err error
	run.ScheduledFor, err = parseTimePtr(scheduledForRaw)
	if err != nil {
		return domain.Run{}, fmt.Errorf("parse scheduled_for for run %d: %w", run.ID, err)
	}

	run.QueuedAt, err = parseTime(queuedAtRaw)
	if err != nil {
		return domain.Run{}, fmt.Errorf("parse queued_at for run %d: %w", run.ID, err)
	}

	run.StartedAt, err = parseTimePtr(startedAtRaw)
	if err != nil {
		return domain.Run{}, fmt.Errorf("parse started_at for run %d: %w", run.ID, err)
	}

	run.FinishedAt, err = parseTimePtr(finishedAtRaw)
	if err != nil {
		return domain.Run{}, fmt.Errorf("parse finished_at for run %d: %w", run.ID, err)
	}

	if exitCodeRaw.Valid {
		exitCode := int(exitCodeRaw.Int64)
		run.ExitCode = &exitCode
	}

	if errorMessageRaw.Valid {
		run.ErrorMessage = stringPtr(errorMessageRaw.String)
	}

	if runnerIDRaw.Valid {
		run.RunnerID = stringPtr(runnerIDRaw.String)
	}

	return run, nil
}

func nullIntToBool(value sql.NullInt64, field string) (bool, error) {
	if !value.Valid {
		return false, fmt.Errorf("%s is NULL", field)
	}

	return intToBool(int(value.Int64))
}

func intPtrValue(value *int) any {
	if value == nil {
		return nil
	}

	return *value
}

func stringPtrValue(value *string) any {
	if value == nil {
		return nil
	}

	return *value
}

func stringPtr(value string) *string {
	return &value
}
