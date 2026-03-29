package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/schedule"
)

var currentTime = func() time.Time {
	return time.Now().UTC()
}

type jobSummaryOutput struct {
	Name              string  `json:"name"`
	Enabled           bool    `json:"enabled"`
	Schedule          string  `json:"schedule"`
	Timezone          string  `json:"timezone"`
	ConcurrencyPolicy string  `json:"concurrency_policy"`
	NextRunAt         *string `json:"next_run_at"`
	LastRunAt         *string `json:"last_run_at"`
	LastRunStatus     *string `json:"last_run_status"`
}

type jobDetailOutput struct {
	ID                int64   `json:"id"`
	Name              string  `json:"name"`
	Command           string  `json:"command"`
	Schedule          string  `json:"schedule"`
	Timezone          string  `json:"timezone"`
	Enabled           bool    `json:"enabled"`
	ConcurrencyPolicy string  `json:"concurrency_policy"`
	NextRunAt         *string `json:"next_run_at"`
	LastRunAt         *string `json:"last_run_at"`
	LastRunStatus     *string `json:"last_run_status"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type deleteResultOutput struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

type runEnqueueOutput struct {
	RunID       int64  `json:"run_id"`
	Job         string `json:"job"`
	Status      string `json:"status"`
	TriggerType string `json:"trigger_type"`
	QueuedAt    string `json:"queued_at"`
}

type runSummaryOutput struct {
	ID         int64   `json:"id"`
	Job        string  `json:"job"`
	Trigger    string  `json:"trigger"`
	Status     string  `json:"status"`
	QueuedAt   string  `json:"queued_at"`
	StartedAt  *string `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	Duration   string  `json:"duration"`
}

type runOutputDetail struct {
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	UpdatedAt       string `json:"updated_at"`
}

type runDetailOutput struct {
	ID              int64            `json:"id"`
	Job             string           `json:"job"`
	JobID           int64            `json:"job_id"`
	TriggerType     string           `json:"trigger_type"`
	Status          string           `json:"status"`
	ScheduledFor    *string          `json:"scheduled_for"`
	QueuedAt        string           `json:"queued_at"`
	StartedAt       *string          `json:"started_at"`
	FinishedAt      *string          `json:"finished_at"`
	Duration        string           `json:"duration"`
	ExitCode        *int             `json:"exit_code"`
	ErrorMessage    *string          `json:"error_message"`
	RunnerID        *string          `json:"runner_id"`
	StdoutTruncated bool             `json:"stdout_truncated"`
	StderrTruncated bool             `json:"stderr_truncated"`
	StdoutPreview   string           `json:"stdout_preview"`
	StderrPreview   string           `json:"stderr_preview"`
	OutputUpdatedAt *string          `json:"output_updated_at"`
	Output          *runOutputDetail `json:"output"`
}

type fieldValue struct {
	Field string
	Value string
}

func normalizedTimezone(timezone string) string {
	if timezone == "" {
		return "Local"
	}

	return timezone
}

func loadLocation(timezone string) (*time.Location, error) {
	normalized := normalizedTimezone(timezone)
	if normalized == "Local" {
		return time.Local, nil
	}

	location, err := time.LoadLocation(normalized)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", normalized, err)
	}

	return location, nil
}

func computeNextRun(spec domain.Schedule, timezone string, refNow time.Time) (*time.Time, error) {
	location, err := loadLocation(timezone)
	if err != nil {
		return nil, err
	}

	nextRunAt, err := schedule.Next(spec, refNow.UTC(), location)
	if err != nil {
		return nil, err
	}

	return &nextRunAt, nil
}

func formatTimeRFC3339(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func formatTimePtrRFC3339(value *time.Time) *string {
	if value == nil {
		return nil
	}

	formatted := formatTimeRFC3339(*value)
	return &formatted
}

func formatDuration(startedAt *time.Time, finishedAt *time.Time) string {
	if startedAt == nil || finishedAt == nil {
		return ""
	}

	return finishedAt.Sub(*startedAt).String()
}

func previewOutput(text string, max int) string {
	escaped := strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(text)
	if max <= 0 || len(escaped) <= max {
		return escaped
	}

	return escaped[:max] + "..."
}

func fieldValueRows(values ...fieldValue) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value.Field, value.Value})
	}

	return rows
}

func boolString(value bool) string {
	return strconv.FormatBool(value)
}

func intPtrString(value *int) string {
	if value == nil {
		return ""
	}

	return strconv.Itoa(*value)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func jobSummaryFromDomain(job domain.Job) jobSummaryOutput {
	return jobSummaryOutput{
		Name:              job.Name,
		Enabled:           job.Enabled,
		Schedule:          job.ScheduleExpr,
		Timezone:          normalizedTimezone(job.Timezone),
		ConcurrencyPolicy: string(job.ConcurrencyPolicy),
		NextRunAt:         formatTimePtrRFC3339(job.NextRunAt),
		LastRunAt:         formatTimePtrRFC3339(job.LastRunAt),
		LastRunStatus:     runStatusStringPtr(job.LastRunStatus),
	}
}

func jobDetailFromDomain(job domain.Job) jobDetailOutput {
	return jobDetailOutput{
		ID:                job.ID,
		Name:              job.Name,
		Command:           job.Command,
		Schedule:          job.ScheduleExpr,
		Timezone:          normalizedTimezone(job.Timezone),
		Enabled:           job.Enabled,
		ConcurrencyPolicy: string(job.ConcurrencyPolicy),
		NextRunAt:         formatTimePtrRFC3339(job.NextRunAt),
		LastRunAt:         formatTimePtrRFC3339(job.LastRunAt),
		LastRunStatus:     runStatusStringPtr(job.LastRunStatus),
		CreatedAt:         formatTimeRFC3339(job.CreatedAt),
		UpdatedAt:         formatTimeRFC3339(job.UpdatedAt),
	}
}

func runEnqueueFromDomain(jobName string, run domain.Run) runEnqueueOutput {
	return runEnqueueOutput{
		RunID:       run.ID,
		Job:         jobName,
		Status:      string(run.Status),
		TriggerType: string(run.TriggerType),
		QueuedAt:    formatTimeRFC3339(run.QueuedAt),
	}
}

func runSummaryFromDomain(run domain.Run) runSummaryOutput {
	return runSummaryOutput{
		ID:         run.ID,
		Job:        run.JobName,
		Trigger:    string(run.TriggerType),
		Status:     string(run.Status),
		QueuedAt:   formatTimeRFC3339(run.QueuedAt),
		StartedAt:  formatTimePtrRFC3339(run.StartedAt),
		FinishedAt: formatTimePtrRFC3339(run.FinishedAt),
		Duration:   formatDuration(run.StartedAt, run.FinishedAt),
	}
}

func runDetailFromDomain(run domain.Run) runDetailOutput {
	detail := runDetailOutput{
		ID:           run.ID,
		Job:          run.JobName,
		JobID:        run.JobID,
		TriggerType:  string(run.TriggerType),
		Status:       string(run.Status),
		ScheduledFor: formatTimePtrRFC3339(run.ScheduledFor),
		QueuedAt:     formatTimeRFC3339(run.QueuedAt),
		StartedAt:    formatTimePtrRFC3339(run.StartedAt),
		FinishedAt:   formatTimePtrRFC3339(run.FinishedAt),
		Duration:     formatDuration(run.StartedAt, run.FinishedAt),
		ExitCode:     run.ExitCode,
		ErrorMessage: run.ErrorMessage,
		RunnerID:     run.RunnerID,
	}

	if run.Output != nil {
		updatedAt := formatTimeRFC3339(run.Output.UpdatedAt)
		detail.StdoutTruncated = run.Output.StdoutTruncated
		detail.StderrTruncated = run.Output.StderrTruncated
		detail.StdoutPreview = previewOutput(run.Output.Stdout, 120)
		detail.StderrPreview = previewOutput(run.Output.Stderr, 120)
		detail.OutputUpdatedAt = &updatedAt
		detail.Output = &runOutputDetail{
			Stdout:          run.Output.Stdout,
			Stderr:          run.Output.Stderr,
			StdoutTruncated: run.Output.StdoutTruncated,
			StderrTruncated: run.Output.StderrTruncated,
			UpdatedAt:       updatedAt,
		}
	}

	return detail
}

func runStatusStringPtr(status *domain.RunStatus) *string {
	if status == nil {
		return nil
	}

	value := string(*status)
	return &value
}
