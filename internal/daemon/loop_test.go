package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/sqlite"
)

func TestLoopProcessesDueIntervalJob(t *testing.T) {
	now := time.Date(2025, 4, 10, 10, 25, 0, 0, time.UTC)
	job := loopTestJob(1, "interval", domain.ConcurrencyPolicyQueue, "every 10m", domain.ScheduleKindInterval)
	dueAt := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	job.NextRunAt = &dueAt

	jobStore := newFakeLoopJobStore(job)
	runStore := newFakeLoopRunStore()
	executor := &fakeExecutor{
		execute: func(ctx context.Context, command string) ExecutionResult {
			return ExecutionResult{
				Status:     domain.RunStatusSucceeded,
				StartedAt:  now,
				FinishedAt: now.Add(1 * time.Second),
				ExitCode:   intPointer(0),
			}
		},
	}

	tick := make(chan time.Time, 1)
	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startTestLoop(loopCtx, jobStore, runStore, executor, tick, func() time.Time { return now })

	tick <- now

	waitForLoop(t, errCh, func() bool {
		updated, _ := jobStore.GetByID(context.Background(), job.ID)
		runs := runStore.runsByJob(job.ID)
		return updated.NextRunAt != nil && updated.NextRunAt.Equal(time.Date(2025, 4, 10, 10, 30, 0, 0, time.UTC)) && len(runs) == 1 && runs[0].Status == domain.RunStatusSucceeded
	})

	updated, err := jobStore.GetByID(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.NextRunAt == nil || !updated.NextRunAt.Equal(time.Date(2025, 4, 10, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("NextRunAt = %v, want 2025-04-10 10:30:00 +0000 UTC", updated.NextRunAt)
	}
	if updated.LastRunStatus == nil || *updated.LastRunStatus != domain.RunStatusSucceeded {
		t.Fatalf("LastRunStatus = %v, want %q", updated.LastRunStatus, domain.RunStatusSucceeded)
	}
}

func TestLoopProcessesDueCronJobWithTimezone(t *testing.T) {
	now := time.Date(2025, 4, 10, 0, 0, 0, 0, time.UTC)
	job := loopTestJob(1, "cron", domain.ConcurrencyPolicyQueue, "cron 0 9 * * *", domain.ScheduleKindCron)
	job.Timezone = "Asia/Seoul"
	dueAt := now
	job.NextRunAt = &dueAt

	jobStore := newFakeLoopJobStore(job)
	runStore := newFakeLoopRunStore()
	executor := &fakeExecutor{
		execute: func(ctx context.Context, command string) ExecutionResult {
			return ExecutionResult{
				Status:     domain.RunStatusSucceeded,
				StartedAt:  now,
				FinishedAt: now.Add(1 * time.Second),
				ExitCode:   intPointer(0),
			}
		},
	}

	tick := make(chan time.Time, 1)
	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startTestLoop(loopCtx, jobStore, runStore, executor, tick, func() time.Time { return now })
	tick <- now

	waitForLoop(t, errCh, func() bool {
		updated, _ := jobStore.GetByID(context.Background(), job.ID)
		return updated.NextRunAt != nil && updated.NextRunAt.Equal(time.Date(2025, 4, 11, 0, 0, 0, 0, time.UTC))
	})
}

func TestLoopConsumesOneTimeJobsOnEnqueueAndSkip(t *testing.T) {
	t.Run("enqueue", func(t *testing.T) {
		now := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
		job := loopTestJob(1, "once", domain.ConcurrencyPolicyQueue, "after 10m", domain.ScheduleKindOnce)
		job.NextRunAt = &now

		jobStore := newFakeLoopJobStore(job)
		runStore := newFakeLoopRunStore()
		executor := &fakeExecutor{
			execute: func(ctx context.Context, command string) ExecutionResult {
				return ExecutionResult{
					Status:     domain.RunStatusSucceeded,
					StartedAt:  now,
					FinishedAt: now.Add(1 * time.Second),
					ExitCode:   intPointer(0),
				}
			},
		}

		tick := make(chan time.Time, 1)
		loopCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := startTestLoop(loopCtx, jobStore, runStore, executor, tick, func() time.Time { return now })
		tick <- now

		waitForLoop(t, errCh, func() bool {
			updated, _ := jobStore.GetByID(context.Background(), job.ID)
			runs := runStore.runsByJob(job.ID)
			return !updated.Enabled && updated.NextRunAt == nil && len(runs) == 1 && runs[0].Status == domain.RunStatusSucceeded
		})
	})

	t.Run("forbid skip", func(t *testing.T) {
		now := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
		job := loopTestJob(1, "once-skip", domain.ConcurrencyPolicyForbid, "after 10m", domain.ScheduleKindOnce)
		job.NextRunAt = &now

		jobStore := newFakeLoopJobStore(job)
		runStore := newFakeLoopRunStore()
		pending := domain.Run{
			ID:          1,
			JobID:       job.ID,
			JobName:     job.Name,
			TriggerType: domain.RunTriggerTypeManual,
			Status:      domain.RunStatusPending,
			QueuedAt:    now.Add(-1 * time.Minute),
		}
		runStore.addRun(pending)

		tick := make(chan time.Time, 1)
		loopCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := startTestLoop(loopCtx, jobStore, runStore, &fakeExecutor{}, tick, func() time.Time { return now })
		tick <- now

		waitForLoop(t, errCh, func() bool {
			updated, _ := jobStore.GetByID(context.Background(), job.ID)
			return !updated.Enabled && updated.NextRunAt == nil && len(runStore.runsByJob(job.ID)) == 1
		})
	})
}

func TestLoopRunsManualPendingAndPersistsFailure(t *testing.T) {
	now := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	job := loopTestJob(1, "manual", domain.ConcurrencyPolicyQueue, "every 10m", domain.ScheduleKindInterval)
	jobStore := newFakeLoopJobStore(job)
	runStore := newFakeLoopRunStore()
	runStore.addRun(domain.Run{
		ID:          1,
		JobID:       job.ID,
		JobName:     job.Name,
		TriggerType: domain.RunTriggerTypeManual,
		Status:      domain.RunStatusPending,
		QueuedAt:    now.Add(-1 * time.Minute),
	})

	executor := &fakeExecutor{
		execute: func(ctx context.Context, command string) ExecutionResult {
			return ExecutionResult{
				Status:       domain.RunStatusFailed,
				StartedAt:    now,
				FinishedAt:   now.Add(2 * time.Second),
				ExitCode:     intPointer(7),
				ErrorMessage: stringPointer("boom"),
				Output: &domain.RunOutput{
					Stderr:    "boom",
					UpdatedAt: now.Add(2 * time.Second),
				},
			}
		},
	}

	tick := make(chan time.Time, 1)
	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startTestLoop(loopCtx, jobStore, runStore, executor, tick, func() time.Time { return now })
	tick <- now

	waitForLoop(t, errCh, func() bool {
		run, _ := runStore.Get(context.Background(), 1)
		return run.Status == domain.RunStatusFailed && run.Output != nil
	})

	run, err := runStore.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if run.Status != domain.RunStatusFailed {
		t.Fatalf("Status = %q, want %q", run.Status, domain.RunStatusFailed)
	}
	if run.ExitCode == nil || *run.ExitCode != 7 {
		t.Fatalf("ExitCode = %v, want 7", run.ExitCode)
	}
	if run.ErrorMessage == nil || *run.ErrorMessage != "boom" {
		t.Fatalf("ErrorMessage = %v, want boom", run.ErrorMessage)
	}
	if run.Output == nil || run.Output.Stderr != "boom" {
		t.Fatalf("Output = %#v, want stderr boom", run.Output)
	}

	updated, err := jobStore.GetByID(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.LastRunStatus == nil || *updated.LastRunStatus != domain.RunStatusFailed {
		t.Fatalf("LastRunStatus = %v, want %q", updated.LastRunStatus, domain.RunStatusFailed)
	}
}

func TestLoopQueueSerializesExecutionPerJob(t *testing.T) {
	now := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	job := loopTestJob(1, "queue", domain.ConcurrencyPolicyQueue, "every 10m", domain.ScheduleKindInterval)
	jobStore := newFakeLoopJobStore(job)
	runStore := newFakeLoopRunStore()
	runStore.addRun(domain.Run{
		ID:          1,
		JobID:       job.ID,
		JobName:     job.Name,
		TriggerType: domain.RunTriggerTypeManual,
		Status:      domain.RunStatusPending,
		QueuedAt:    now.Add(-2 * time.Minute),
	})
	runStore.addRun(domain.Run{
		ID:          2,
		JobID:       job.ID,
		JobName:     job.Name,
		TriggerType: domain.RunTriggerTypeManual,
		Status:      domain.RunStatusPending,
		QueuedAt:    now.Add(-1 * time.Minute),
	})

	releaseFirst := make(chan struct{})
	var (
		callIndex int
		callMu    sync.Mutex
	)
	executor := &fakeExecutor{
		execute: func(ctx context.Context, command string) ExecutionResult {
			callMu.Lock()
			callIndex++
			currentCall := callIndex
			callMu.Unlock()
			if currentCall == 1 {
				<-releaseFirst
				return ExecutionResult{
					Status:     domain.RunStatusSucceeded,
					StartedAt:  now,
					FinishedAt: now.Add(1 * time.Second),
					ExitCode:   intPointer(0),
				}
			}
			return ExecutionResult{
				Status:     domain.RunStatusSucceeded,
				StartedAt:  now.Add(2 * time.Second),
				FinishedAt: now.Add(3 * time.Second),
				ExitCode:   intPointer(0),
			}
		},
	}

	tick := make(chan time.Time, 2)
	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startTestLoop(loopCtx, jobStore, runStore, executor, tick, func() time.Time { return now })
	tick <- now

	waitForLoop(t, errCh, func() bool {
		first, _ := runStore.Get(context.Background(), 1)
		second, _ := runStore.Get(context.Background(), 2)
		return first.Status == domain.RunStatusRunning && second.Status == domain.RunStatusPending
	})

	close(releaseFirst)

	waitForLoop(t, errCh, func() bool {
		first, _ := runStore.Get(context.Background(), 1)
		return first.Status == domain.RunStatusSucceeded
	})

	tick <- now.Add(5 * time.Second)

	waitForLoop(t, errCh, func() bool {
		second, _ := runStore.Get(context.Background(), 2)
		return second.Status == domain.RunStatusSucceeded
	})
}

func TestLoopForbidSkipAdvancesSchedule(t *testing.T) {
	now := time.Date(2025, 4, 10, 10, 25, 0, 0, time.UTC)
	job := loopTestJob(1, "forbid", domain.ConcurrencyPolicyForbid, "every 10m", domain.ScheduleKindInterval)
	dueAt := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	job.NextRunAt = &dueAt

	jobStore := newFakeLoopJobStore(job)
	runStore := newFakeLoopRunStore()
	runStore.addRun(domain.Run{
		ID:          1,
		JobID:       job.ID,
		JobName:     job.Name,
		TriggerType: domain.RunTriggerTypeManual,
		Status:      domain.RunStatusPending,
		QueuedAt:    now.Add(-1 * time.Minute),
	})

	tick := make(chan time.Time, 1)
	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := startTestLoop(loopCtx, jobStore, runStore, &fakeExecutor{}, tick, func() time.Time { return now })
	tick <- now

	waitForLoop(t, errCh, func() bool {
		updated, _ := jobStore.GetByID(context.Background(), job.ID)
		return updated.NextRunAt != nil && updated.NextRunAt.Equal(time.Date(2025, 4, 10, 10, 30, 0, 0, time.UTC))
	})

	runs := runStore.runsByJob(job.ID)
	if len(runs) != 1 {
		t.Fatalf("runs length = %d, want 1", len(runs))
	}
}

func TestLoopReplaceCancelsPendingAndRunningRuns(t *testing.T) {
	now := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	job := loopTestJob(1, "replace", domain.ConcurrencyPolicyReplace, "every 10m", domain.ScheduleKindInterval)
	firstDue := now.Add(5 * time.Minute)
	job.NextRunAt = &firstDue

	jobStore := newFakeLoopJobStore(job)
	runStore := newFakeLoopRunStore()
	runStore.addRun(domain.Run{
		ID:          1,
		JobID:       job.ID,
		JobName:     job.Name,
		TriggerType: domain.RunTriggerTypeManual,
		Status:      domain.RunStatusPending,
		QueuedAt:    now.Add(-2 * time.Minute),
	})

	var (
		callIndex int
		callMu    sync.Mutex
	)
	executor := &fakeExecutor{
		execute: func(ctx context.Context, command string) ExecutionResult {
			callMu.Lock()
			callIndex++
			currentCall := callIndex
			callMu.Unlock()
			if currentCall == 1 {
				<-ctx.Done()
				return ExecutionResult{
					Status:       domain.RunStatusFailed,
					StartedAt:    now,
					FinishedAt:   now.Add(6 * time.Minute),
					ErrorMessage: stringPointer(ctx.Err().Error()),
					Output: &domain.RunOutput{
						Stdout:    "partial",
						UpdatedAt: now.Add(6 * time.Minute),
					},
				}
			}
			return ExecutionResult{
				Status:     domain.RunStatusSucceeded,
				StartedAt:  now.Add(6 * time.Minute),
				FinishedAt: now.Add(7 * time.Minute),
				ExitCode:   intPointer(0),
			}
		},
	}

	tick := make(chan time.Time, 2)
	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	currentNow := now
	errCh := startTestLoop(loopCtx, jobStore, runStore, executor, tick, func() time.Time { return currentNow })

	tick <- now
	waitForLoop(t, errCh, func() bool {
		run, _ := runStore.Get(context.Background(), 1)
		return run.Status == domain.RunStatusRunning
	})

	currentNow = firstDue
	tick <- firstDue

	waitForLoop(t, errCh, func() bool {
		runs := runStore.runsByJob(job.ID)
		return len(runs) == 2 && runs[0].Status == domain.RunStatusCanceled && runs[1].Status == domain.RunStatusSucceeded
	})

	runs := runStore.runsByJob(job.ID)
	if runs[0].ErrorMessage == nil || *runs[0].ErrorMessage != "canceled by replacement" {
		t.Fatalf("first ErrorMessage = %v, want canceled by replacement", runs[0].ErrorMessage)
	}
	if runs[0].Output == nil || runs[0].Output.Stdout != "partial" {
		t.Fatalf("first Output = %#v, want partial stdout", runs[0].Output)
	}
}

func TestLoopShutdownCancelsActiveRuns(t *testing.T) {
	now := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	job := loopTestJob(1, "shutdown", domain.ConcurrencyPolicyQueue, "every 10m", domain.ScheduleKindInterval)
	jobStore := newFakeLoopJobStore(job)
	runStore := newFakeLoopRunStore()
	runStore.addRun(domain.Run{
		ID:          1,
		JobID:       job.ID,
		JobName:     job.Name,
		TriggerType: domain.RunTriggerTypeManual,
		Status:      domain.RunStatusPending,
		QueuedAt:    now.Add(-1 * time.Minute),
	})

	executor := &fakeExecutor{
		execute: func(ctx context.Context, command string) ExecutionResult {
			<-ctx.Done()
			return ExecutionResult{
				Status:       domain.RunStatusFailed,
				StartedAt:    now,
				FinishedAt:   now.Add(1 * time.Second),
				ErrorMessage: stringPointer(ctx.Err().Error()),
			}
		},
	}

	tick := make(chan time.Time, 1)
	loopCtx, cancel := context.WithCancel(context.Background())
	errCh := startTestLoop(loopCtx, jobStore, runStore, executor, tick, func() time.Time { return now })

	tick <- now
	waitForLoop(t, errCh, func() bool {
		run, _ := runStore.Get(context.Background(), 1)
		return run.Status == domain.RunStatusRunning
	})

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not exit after shutdown")
	}

	run, err := runStore.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if run.Status != domain.RunStatusCanceled {
		t.Fatalf("Status = %q, want %q", run.Status, domain.RunStatusCanceled)
	}
	if run.ErrorMessage == nil || *run.ErrorMessage != "canceled by shutdown" {
		t.Fatalf("ErrorMessage = %v, want canceled by shutdown", run.ErrorMessage)
	}
}

func startTestLoop(
	ctx context.Context,
	jobStore *fakeLoopJobStore,
	runStore *fakeLoopRunStore,
	executor Executor,
	tick <-chan time.Time,
	now func() time.Time,
) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- (&Loop{
			JobStore:   jobStore,
			RunStore:   runStore,
			Executor:   executor,
			Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			Now:        now,
			Tick:       tick,
			ClaimLimit: 64,
		}).Run(ctx)
	}()
	return errCh
}

func waitForLoop(t *testing.T, errCh <-chan error, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("Run() exited early: %v", err)
		default:
		}

		if condition() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition was not satisfied before timeout")
}

func loopTestJob(
	id int64,
	name string,
	policy domain.ConcurrencyPolicy,
	expr string,
	kind domain.ScheduleKind,
) domain.Job {
	base := time.Date(2025, 4, 10, 9, 0, 0, 0, time.UTC)
	return domain.Job{
		ID:                id,
		Name:              name,
		Command:           name,
		ScheduleKind:      kind,
		ScheduleExpr:      expr,
		Timezone:          "UTC",
		Enabled:           true,
		ConcurrencyPolicy: policy,
		CreatedAt:         base,
		UpdatedAt:         base,
	}
}

type fakeLoopJobStore struct {
	mu   sync.Mutex
	jobs map[int64]domain.Job
}

func newFakeLoopJobStore(jobs ...domain.Job) *fakeLoopJobStore {
	store := &fakeLoopJobStore{
		jobs: make(map[int64]domain.Job, len(jobs)),
	}
	for _, job := range jobs {
		store.jobs[job.ID] = job
	}
	return store
}

func (s *fakeLoopJobStore) GetByID(ctx context.Context, id int64) (domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return domain.Job{}, errors.New("job not found")
	}
	return cloneJob(job), nil
}

func (s *fakeLoopJobStore) ListDue(ctx context.Context, now time.Time) ([]domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs := make([]domain.Job, 0)
	for _, job := range s.jobs {
		if !job.Enabled || job.NextRunAt == nil || job.NextRunAt.After(now) {
			continue
		}
		jobs = append(jobs, cloneJob(job))
	}

	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].NextRunAt.Equal(*jobs[j].NextRunAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].NextRunAt.Before(*jobs[j].NextRunAt)
	})

	return jobs, nil
}

func (s *fakeLoopJobStore) Update(ctx context.Context, job domain.Job) (domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.jobs[job.ID] = cloneJob(job)
	return cloneJob(job), nil
}

func (s *fakeLoopJobStore) UpdateNextRun(ctx context.Context, jobID int64, nextRunAt *time.Time, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return errors.New("job not found")
	}
	job.NextRunAt = cloneTimePtr(nextRunAt)
	job.UpdatedAt = updatedAt
	s.jobs[jobID] = job
	return nil
}

func (s *fakeLoopJobStore) UpdateLastRunSummary(
	ctx context.Context,
	jobID int64,
	lastRunAt *time.Time,
	lastRunStatus *domain.RunStatus,
	updatedAt time.Time,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return errors.New("job not found")
	}
	job.LastRunAt = cloneTimePtr(lastRunAt)
	if lastRunStatus != nil {
		status := *lastRunStatus
		job.LastRunStatus = &status
	} else {
		job.LastRunStatus = nil
	}
	job.UpdatedAt = updatedAt
	s.jobs[jobID] = job
	return nil
}

type fakeLoopRunStore struct {
	mu     sync.Mutex
	nextID int64
	runs   map[int64]domain.Run
}

func newFakeLoopRunStore() *fakeLoopRunStore {
	return &fakeLoopRunStore{
		nextID: 1,
		runs:   make(map[int64]domain.Run),
	}
}

func (s *fakeLoopRunStore) EnqueueManual(ctx context.Context, jobID int64, queuedAt time.Time) (domain.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	run := domain.Run{
		ID:          s.allocateID(),
		JobID:       jobID,
		TriggerType: domain.RunTriggerTypeManual,
		Status:      domain.RunStatusPending,
		QueuedAt:    queuedAt,
	}
	s.runs[run.ID] = cloneRun(run)
	return cloneRun(run), nil
}

func (s *fakeLoopRunStore) EnqueueScheduled(
	ctx context.Context,
	jobID int64,
	scheduledFor time.Time,
	queuedAt time.Time,
) (domain.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	run := domain.Run{
		ID:           s.allocateID(),
		JobID:        jobID,
		TriggerType:  domain.RunTriggerTypeSchedule,
		Status:       domain.RunStatusPending,
		ScheduledFor: cloneTimePtr(&scheduledFor),
		QueuedAt:     queuedAt,
	}
	s.runs[run.ID] = cloneRun(run)
	return cloneRun(run), nil
}

func (s *fakeLoopRunStore) CancelPendingByJob(ctx context.Context, jobID int64, canceledAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, run := range s.runs {
		if run.JobID != jobID || run.Status != domain.RunStatusPending {
			continue
		}
		run.Status = domain.RunStatusCanceled
		run.FinishedAt = cloneTimePtr(&canceledAt)
		run.RunnerID = nil
		s.runs[id] = cloneRun(run)
	}

	return nil
}

func (s *fakeLoopRunStore) ListUnfinishedByJob(ctx context.Context, jobID int64) ([]domain.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	runs := make([]domain.Run, 0)
	for _, run := range s.runs {
		if run.JobID != jobID {
			continue
		}
		if run.Status != domain.RunStatusPending && run.Status != domain.RunStatusRunning {
			continue
		}
		runs = append(runs, cloneRun(run))
	}
	sortRunsByQueuedAt(runs)
	return runs, nil
}

func (s *fakeLoopRunStore) ListPending(ctx context.Context, limit int) ([]domain.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	runs := make([]domain.Run, 0)
	for _, run := range s.runs {
		if run.Status != domain.RunStatusPending || run.RunnerID != nil {
			continue
		}
		runs = append(runs, cloneRun(run))
	}
	sortRunsByQueuedAt(runs)
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

func (s *fakeLoopRunStore) TryClaimPending(ctx context.Context, runID int64, runnerID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	run, ok := s.runs[runID]
	if !ok {
		return false, nil
	}
	if run.Status != domain.RunStatusPending || run.RunnerID != nil {
		return false, nil
	}
	run.RunnerID = stringPointer(runnerID)
	s.runs[runID] = cloneRun(run)
	return true, nil
}

func (s *fakeLoopRunStore) MarkRunning(ctx context.Context, runID int64, startedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	run, ok := s.runs[runID]
	if !ok {
		return sqlite.ErrRunNotFound
	}
	run.Status = domain.RunStatusRunning
	run.StartedAt = cloneTimePtr(&startedAt)
	s.runs[runID] = cloneRun(run)
	return nil
}

func (s *fakeLoopRunStore) MarkFinished(ctx context.Context, params sqlite.FinishRunParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	run, ok := s.runs[params.RunID]
	if !ok {
		return sqlite.ErrRunNotFound
	}
	run.Status = params.Status
	run.FinishedAt = cloneTimePtr(&params.FinishedAt)
	run.ExitCode = cloneIntPtr(params.ExitCode)
	run.ErrorMessage = cloneStringPtr(params.ErrorMessage)
	run.RunnerID = nil
	if params.Output != nil {
		run.Output = cloneRunOutput(params.Output)
	}
	s.runs[params.RunID] = cloneRun(run)
	return nil
}

func (s *fakeLoopRunStore) Get(ctx context.Context, runID int64) (domain.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	run, ok := s.runs[runID]
	if !ok {
		return domain.Run{}, sqlite.ErrRunNotFound
	}
	return cloneRun(run), nil
}

func (s *fakeLoopRunStore) addRun(run domain.Run) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.runs[run.ID] = cloneRun(run)
	if run.ID >= s.nextID {
		s.nextID = run.ID + 1
	}
}

func (s *fakeLoopRunStore) runsByJob(jobID int64) []domain.Run {
	s.mu.Lock()
	defer s.mu.Unlock()

	runs := make([]domain.Run, 0)
	for _, run := range s.runs {
		if run.JobID == jobID {
			runs = append(runs, cloneRun(run))
		}
	}
	sortRunsByID(runs)
	return runs
}

func (s *fakeLoopRunStore) allocateID() int64 {
	id := s.nextID
	s.nextID++
	return id
}

type fakeExecutor struct {
	execute func(ctx context.Context, command string) ExecutionResult
}

func (e *fakeExecutor) Execute(ctx context.Context, command string) ExecutionResult {
	if e.execute == nil {
		return ExecutionResult{
			Status:     domain.RunStatusSucceeded,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			ExitCode:   intPointer(0),
		}
	}
	return e.execute(ctx, command)
}

func sortRunsByQueuedAt(runs []domain.Run) {
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].QueuedAt.Equal(runs[j].QueuedAt) {
			return runs[i].ID < runs[j].ID
		}
		return runs[i].QueuedAt.Before(runs[j].QueuedAt)
	})
}

func sortRunsByID(runs []domain.Run) {
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].ID < runs[j].ID
	})
}

func cloneJob(job domain.Job) domain.Job {
	job.NextRunAt = cloneTimePtr(job.NextRunAt)
	job.LastRunAt = cloneTimePtr(job.LastRunAt)
	if job.LastRunStatus != nil {
		status := *job.LastRunStatus
		job.LastRunStatus = &status
	}
	return job
}

func cloneRun(run domain.Run) domain.Run {
	run.ScheduledFor = cloneTimePtr(run.ScheduledFor)
	run.StartedAt = cloneTimePtr(run.StartedAt)
	run.FinishedAt = cloneTimePtr(run.FinishedAt)
	run.ExitCode = cloneIntPtr(run.ExitCode)
	run.ErrorMessage = cloneStringPtr(run.ErrorMessage)
	run.RunnerID = cloneStringPtr(run.RunnerID)
	run.Output = cloneRunOutput(run.Output)
	return run
}

func cloneRunOutput(output *domain.RunOutput) *domain.RunOutput {
	if output == nil {
		return nil
	}
	cloned := *output
	return &cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
