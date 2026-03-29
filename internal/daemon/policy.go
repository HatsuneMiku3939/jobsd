package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/schedule"
)

var ErrRunConflict = errors.New("run conflict")

type policyJobStore interface {
	Update(ctx context.Context, job domain.Job) (domain.Job, error)
	UpdateNextRun(ctx context.Context, jobID int64, nextRunAt *time.Time, updatedAt time.Time) error
}

type policyRunStore interface {
	EnqueueManual(ctx context.Context, jobID int64, queuedAt time.Time) (domain.Run, error)
	EnqueueScheduled(ctx context.Context, jobID int64, scheduledFor time.Time, queuedAt time.Time) (domain.Run, error)
	CancelPendingByJob(ctx context.Context, jobID int64, canceledAt time.Time) error
	ListUnfinishedByJob(ctx context.Context, jobID int64) ([]domain.Run, error)
}

func EnqueueScheduledWithPolicy(
	ctx context.Context,
	jobStore policyJobStore,
	runStore policyRunStore,
	job domain.Job,
	now time.Time,
) error {
	if job.NextRunAt == nil {
		return fmt.Errorf("job %d next run is required for scheduled enqueue", job.ID)
	}

	switch normalizePolicy(job.ConcurrencyPolicy) {
	case domain.ConcurrencyPolicyForbid:
		unfinished, err := runStore.ListUnfinishedByJob(ctx, job.ID)
		if err != nil {
			return fmt.Errorf("list unfinished runs for job %d: %w", job.ID, err)
		}
		if len(unfinished) == 0 {
			if _, err := runStore.EnqueueScheduled(ctx, job.ID, *job.NextRunAt, now); err != nil {
				return fmt.Errorf("enqueue scheduled run for job %d: %w", job.ID, err)
			}
		}
	case domain.ConcurrencyPolicyQueue:
		if _, err := runStore.EnqueueScheduled(ctx, job.ID, *job.NextRunAt, now); err != nil {
			return fmt.Errorf("enqueue scheduled run for job %d: %w", job.ID, err)
		}
	case domain.ConcurrencyPolicyReplace:
		if err := runStore.CancelPendingByJob(ctx, job.ID, now); err != nil {
			return fmt.Errorf("cancel pending runs for job %d: %w", job.ID, err)
		}
		if _, err := runStore.EnqueueScheduled(ctx, job.ID, *job.NextRunAt, now); err != nil {
			return fmt.Errorf("enqueue scheduled run for job %d: %w", job.ID, err)
		}
	default:
		return fmt.Errorf("job %d has unsupported concurrency policy %q", job.ID, job.ConcurrencyPolicy)
	}

	if err := advanceScheduleState(ctx, jobStore, job, now); err != nil {
		return fmt.Errorf("advance schedule state for job %d: %w", job.ID, err)
	}

	return nil
}

func EnqueueManualWithPolicy(
	ctx context.Context,
	runStore policyRunStore,
	job domain.Job,
	now time.Time,
) (domain.Run, error) {
	switch normalizePolicy(job.ConcurrencyPolicy) {
	case domain.ConcurrencyPolicyForbid:
		unfinished, err := runStore.ListUnfinishedByJob(ctx, job.ID)
		if err != nil {
			return domain.Run{}, fmt.Errorf("list unfinished runs for job %d: %w", job.ID, err)
		}
		if len(unfinished) > 0 {
			return domain.Run{}, ErrRunConflict
		}
	case domain.ConcurrencyPolicyReplace:
		if err := runStore.CancelPendingByJob(ctx, job.ID, now); err != nil {
			return domain.Run{}, fmt.Errorf("cancel pending runs for job %d: %w", job.ID, err)
		}
	}

	run, err := runStore.EnqueueManual(ctx, job.ID, now)
	if err != nil {
		return domain.Run{}, fmt.Errorf("enqueue manual run for job %d: %w", job.ID, err)
	}

	return run, nil
}

func advanceScheduleState(ctx context.Context, jobStore policyJobStore, job domain.Job, now time.Time) error {
	updatedAt := now.UTC()

	if job.ScheduleKind == domain.ScheduleKindOnce {
		job.Enabled = false
		job.NextRunAt = nil
		job.UpdatedAt = updatedAt
		if _, err := jobStore.Update(ctx, job); err != nil {
			return err
		}
		return nil
	}

	nextRunAt, err := computeFutureNextRun(job, updatedAt)
	if err != nil {
		return err
	}

	return jobStore.UpdateNextRun(ctx, job.ID, nextRunAt, updatedAt)
}

func computeFutureNextRun(job domain.Job, now time.Time) (*time.Time, error) {
	if job.NextRunAt == nil {
		return nil, fmt.Errorf("job %d next run is required", job.ID)
	}

	location, err := loadJobLocation(job.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone for job %d: %w", job.ID, err)
	}

	spec := domain.Schedule{
		Kind: job.ScheduleKind,
		Expr: job.ScheduleExpr,
	}
	ref := job.NextRunAt.UTC()
	for {
		nextRunAt, err := schedule.Next(spec, ref, location)
		if err != nil {
			return nil, fmt.Errorf("calculate next run for job %d: %w", job.ID, err)
		}
		if nextRunAt.After(now) {
			return &nextRunAt, nil
		}
		ref = nextRunAt
	}
}

func loadJobLocation(timezone string) (*time.Location, error) {
	if timezone == "" || timezone == "Local" {
		return time.Local, nil
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}

	return location, nil
}

func normalizePolicy(policy domain.ConcurrencyPolicy) domain.ConcurrencyPolicy {
	if policy == "" {
		return domain.ConcurrencyPolicyForbid
	}

	return policy
}
