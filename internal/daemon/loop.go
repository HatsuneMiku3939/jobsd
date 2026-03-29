package daemon

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/sqlite"
)

type loopJobStore interface {
	policyJobStore
	GetByID(ctx context.Context, id int64) (domain.Job, error)
	ListDue(ctx context.Context, now time.Time) ([]domain.Job, error)
	UpdateLastRunSummary(
		ctx context.Context,
		jobID int64,
		lastRunAt *time.Time,
		lastRunStatus *domain.RunStatus,
		updatedAt time.Time,
	) error
}

type loopRunStore interface {
	policyRunStore
	ListPending(ctx context.Context, limit int) ([]domain.Run, error)
	TryClaimPending(ctx context.Context, runID int64, runnerID string) (bool, error)
	MarkRunning(ctx context.Context, runID int64, startedAt time.Time) error
	MarkFinished(ctx context.Context, params sqlite.FinishRunParams) error
}

type Loop struct {
	JobStore   loopJobStore
	RunStore   loopRunStore
	Executor   Executor
	Logger     *slog.Logger
	Now        func() time.Time
	Tick       <-chan time.Time
	ClaimLimit int

	mu            sync.Mutex
	wg            sync.WaitGroup
	runnerID      string
	activeByRunID map[int64]*activeRun
}

type cancelReason int

const (
	cancelReasonNone cancelReason = iota
	cancelReasonReplace
	cancelReasonShutdown
)

type activeRun struct {
	RunID  int64
	JobID  int64
	Cancel context.CancelFunc

	mu     sync.Mutex
	Reason cancelReason
}

func (r *activeRun) setReason(reason cancelReason) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Reason == cancelReasonNone {
		r.Reason = reason
	}
}

func (r *activeRun) reason() cancelReason {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.Reason
}

func (l *Loop) Run(ctx context.Context) error {
	if l.JobStore == nil {
		return fmt.Errorf("job store is required")
	}
	if l.RunStore == nil {
		return fmt.Errorf("run store is required")
	}
	if l.Executor == nil {
		return fmt.Errorf("executor is required")
	}

	tickCh, stopTick := l.tickChannel()
	defer stopTick()

	for {
		select {
		case <-ctx.Done():
			l.cancelActiveRuns(cancelReasonShutdown)
			l.wg.Wait()
			return nil
		case _, ok := <-tickCh:
			if !ok {
				l.cancelActiveRuns(cancelReasonShutdown)
				l.wg.Wait()
				return nil
			}

			now := l.now().UTC()
			l.processDueJobs(ctx, now)
			l.processPendingRuns(ctx)
		}
	}
}

func (l *Loop) processDueJobs(ctx context.Context, now time.Time) {
	jobs, err := l.JobStore.ListDue(ctx, now)
	if err != nil {
		l.logger().Error("list due jobs failed", slog.String("error", err.Error()))
		return
	}

	for _, job := range jobs {
		if err := EnqueueScheduledWithPolicy(ctx, l.JobStore, l.RunStore, job, now); err != nil {
			l.logger().Error(
				"process due job failed",
				slog.Int64("job_id", job.ID),
				slog.String("job_name", job.Name),
				slog.String("error", err.Error()),
			)
		}
	}
}

func (l *Loop) processPendingRuns(ctx context.Context) {
	runs, err := l.RunStore.ListPending(ctx, l.claimLimit())
	if err != nil {
		l.logger().Error("list pending runs failed", slog.String("error", err.Error()))
		return
	}

	for _, run := range runs {
		job, err := l.JobStore.GetByID(ctx, run.JobID)
		if err != nil {
			l.logger().Error(
				"load job for pending run failed",
				slog.Int64("run_id", run.ID),
				slog.Int64("job_id", run.JobID),
				slog.String("error", err.Error()),
			)
			continue
		}

		if !l.prepareRunStart(job) {
			continue
		}

		claimed, err := l.RunStore.TryClaimPending(ctx, run.ID, l.ensureRunnerID())
		if err != nil {
			l.logger().Error(
				"claim pending run failed",
				slog.Int64("run_id", run.ID),
				slog.String("error", err.Error()),
			)
			continue
		}
		if !claimed {
			continue
		}

		startedAt := l.now().UTC()
		if err := l.RunStore.MarkRunning(ctx, run.ID, startedAt); err != nil {
			l.logger().Error(
				"mark run running failed",
				slog.Int64("run_id", run.ID),
				slog.String("error", err.Error()),
			)
			continue
		}

		run.Status = domain.RunStatusRunning
		run.StartedAt = &startedAt
		runnerID := l.ensureRunnerID()
		run.RunnerID = &runnerID
		l.startRunExecution(job, run)
	}
}

func (l *Loop) prepareRunStart(job domain.Job) bool {
	switch normalizePolicy(job.ConcurrencyPolicy) {
	case domain.ConcurrencyPolicyForbid, domain.ConcurrencyPolicyQueue:
		return !l.hasActiveRun(job.ID)
	case domain.ConcurrencyPolicyReplace:
		if l.hasActiveRun(job.ID) {
			l.cancelJobRuns(job.ID, cancelReasonReplace)
		}
		return true
	default:
		return !l.hasActiveRun(job.ID)
	}
}

func (l *Loop) startRunExecution(job domain.Job, run domain.Run) {
	execCtx, cancel := context.WithCancel(context.Background())
	active := &activeRun{
		RunID:  run.ID,
		JobID:  job.ID,
		Cancel: cancel,
	}

	l.mu.Lock()
	if l.activeByRunID == nil {
		l.activeByRunID = make(map[int64]*activeRun)
	}
	l.activeByRunID[run.ID] = active
	l.wg.Add(1)
	l.mu.Unlock()

	go func() {
		defer l.finishActiveRun(run.ID)

		result := l.Executor.Execute(execCtx, job.Command)
		l.persistRunCompletion(active, run, result)
	}()
}

func (l *Loop) persistRunCompletion(active *activeRun, run domain.Run, result ExecutionResult) {
	finalStatus := result.Status
	errorMessage := result.ErrorMessage

	switch active.reason() {
	case cancelReasonReplace:
		finalStatus = domain.RunStatusCanceled
		errorMessage = stringPointer("canceled by replacement")
	case cancelReasonShutdown:
		finalStatus = domain.RunStatusCanceled
		errorMessage = stringPointer("canceled by shutdown")
	}

	finishedAt := result.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}

	persistCtx := context.Background()
	if err := l.RunStore.MarkFinished(persistCtx, sqlite.FinishRunParams{
		RunID:        run.ID,
		Status:       finalStatus,
		FinishedAt:   finishedAt,
		ExitCode:     result.ExitCode,
		ErrorMessage: errorMessage,
		Output:       result.Output,
	}); err != nil {
		l.logger().Error(
			"persist finished run failed",
			slog.Int64("run_id", run.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	if err := l.JobStore.UpdateLastRunSummary(
		persistCtx,
		run.JobID,
		&finishedAt,
		&finalStatus,
		finishedAt,
	); err != nil {
		l.logger().Error(
			"update job last run summary failed",
			slog.Int64("job_id", run.JobID),
			slog.Int64("run_id", run.ID),
			slog.String("error", err.Error()),
		)
	}
}

func (l *Loop) cancelActiveRuns(reason cancelReason) {
	l.mu.Lock()
	handles := make([]*activeRun, 0, len(l.activeByRunID))
	for _, active := range l.activeByRunID {
		handles = append(handles, active)
	}
	l.mu.Unlock()

	for _, active := range handles {
		active.setReason(reason)
		active.Cancel()
	}
}

func (l *Loop) cancelJobRuns(jobID int64, reason cancelReason) {
	l.mu.Lock()
	handles := make([]*activeRun, 0, len(l.activeByRunID))
	for _, active := range l.activeByRunID {
		if active.JobID == jobID {
			handles = append(handles, active)
		}
	}
	l.mu.Unlock()

	for _, active := range handles {
		active.setReason(reason)
		active.Cancel()
	}
}

func (l *Loop) hasActiveRun(jobID int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, active := range l.activeByRunID {
		if active.JobID == jobID {
			return true
		}
	}

	return false
}

func (l *Loop) finishActiveRun(runID int64) {
	l.mu.Lock()
	delete(l.activeByRunID, runID)
	l.mu.Unlock()
	l.wg.Done()
}

func (l *Loop) ensureRunnerID() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.runnerID == "" {
		l.runnerID = fmt.Sprintf("loop-%d", time.Now().UnixNano())
	}

	return l.runnerID
}

func (l *Loop) tickChannel() (<-chan time.Time, func()) {
	if l.Tick != nil {
		return l.Tick, func() {}
	}

	ticker := time.NewTicker(1 * time.Second)
	return ticker.C, ticker.Stop
}

func (l *Loop) logger() *slog.Logger {
	if l.Logger != nil {
		return l.Logger
	}

	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func (l *Loop) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}

	return time.Now()
}

func (l *Loop) claimLimit() int {
	if l.ClaimLimit > 0 {
		return l.ClaimLimit
	}

	return 64
}
