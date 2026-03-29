package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestRunStoreEnqueueManualAndScheduled(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	jobStore := NewJobStore(db)
	runStore := NewRunStore(db)

	job, err := jobStore.Create(ctx, testJob("cleanup"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	queuedAt := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	manual, err := runStore.EnqueueManual(ctx, job.ID, queuedAt)
	if err != nil {
		t.Fatalf("EnqueueManual() error = %v", err)
	}
	assertRunCore(t, manual, domain.Run{
		ID:          manual.ID,
		JobID:       job.ID,
		JobName:     job.Name,
		TriggerType: domain.RunTriggerTypeManual,
		Status:      domain.RunStatusPending,
		QueuedAt:    queuedAt,
	})
	if manual.ScheduledFor != nil {
		t.Fatalf("manual ScheduledFor = %v, want nil", manual.ScheduledFor)
	}

	scheduledFor := queuedAt.Add(30 * time.Minute)
	scheduled, err := runStore.EnqueueScheduled(ctx, job.ID, scheduledFor, queuedAt.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("EnqueueScheduled() error = %v", err)
	}
	assertRunCore(t, scheduled, domain.Run{
		ID:           scheduled.ID,
		JobID:        job.ID,
		JobName:      job.Name,
		TriggerType:  domain.RunTriggerTypeSchedule,
		Status:       domain.RunStatusPending,
		ScheduledFor: &scheduledFor,
		QueuedAt:     queuedAt.Add(1 * time.Minute),
	})
}

func TestRunStoreClaimPending(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	jobStore := NewJobStore(db)
	runStore := NewRunStore(db)

	job, err := jobStore.Create(ctx, testJob("cleanup"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	first, err := runStore.EnqueueManual(ctx, job.ID, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("first EnqueueManual() error = %v", err)
	}
	second, err := runStore.EnqueueManual(ctx, job.ID, time.Date(2025, 4, 10, 10, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("second EnqueueManual() error = %v", err)
	}
	third, err := runStore.EnqueueManual(ctx, job.ID, time.Date(2025, 4, 10, 10, 2, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("third EnqueueManual() error = %v", err)
	}

	claimedA, err := runStore.ClaimPending(ctx, "runner-a", 2)
	if err != nil {
		t.Fatalf("ClaimPending() first error = %v", err)
	}
	if len(claimedA) != 2 {
		t.Fatalf("ClaimPending() first length = %d, want 2", len(claimedA))
	}
	if claimedA[0].ID != first.ID || claimedA[1].ID != second.ID {
		t.Fatalf("ClaimPending() first IDs = [%d %d], want [%d %d]", claimedA[0].ID, claimedA[1].ID, first.ID, second.ID)
	}
	for _, run := range claimedA {
		if run.RunnerID == nil || *run.RunnerID != "runner-a" {
			t.Fatalf("ClaimPending() first runner id = %v, want runner-a", run.RunnerID)
		}
	}

	claimedB, err := runStore.ClaimPending(ctx, "runner-b", 2)
	if err != nil {
		t.Fatalf("ClaimPending() second error = %v", err)
	}
	if len(claimedB) != 1 || claimedB[0].ID != third.ID {
		t.Fatalf("ClaimPending() second result = %#v, want [%d]", claimedB, third.ID)
	}
	if claimedB[0].RunnerID == nil || *claimedB[0].RunnerID != "runner-b" {
		t.Fatalf("ClaimPending() second runner id = %v, want runner-b", claimedB[0].RunnerID)
	}

	empty, err := runStore.ClaimPending(ctx, "runner-c", 0)
	if err != nil {
		t.Fatalf("ClaimPending() zero-limit error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ClaimPending() zero-limit length = %d, want 0", len(empty))
	}
}

func TestRunStoreMarkRunningAndFinished(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	jobStore := NewJobStore(db)
	runStore := NewRunStore(db)

	job, err := jobStore.Create(ctx, testJob("cleanup"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	run, err := runStore.EnqueueManual(ctx, job.ID, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EnqueueManual() error = %v", err)
	}

	startedAt := time.Date(2025, 4, 10, 10, 1, 0, 0, time.UTC)
	if err := runStore.MarkRunning(ctx, run.ID, startedAt); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}

	exitCode := 0
	errorMessage := ""
	finishedAt := time.Date(2025, 4, 10, 10, 2, 0, 0, time.UTC)
	firstOutput := &domain.RunOutput{
		Stdout:          "hello",
		Stderr:          "warn",
		StdoutTruncated: false,
		StderrTruncated: true,
		UpdatedAt:       finishedAt,
	}
	if err := runStore.MarkFinished(ctx, FinishRunParams{
		RunID:        run.ID,
		Status:       domain.RunStatusSucceeded,
		FinishedAt:   finishedAt,
		ExitCode:     &exitCode,
		ErrorMessage: &errorMessage,
		Output:       firstOutput,
	}); err != nil {
		t.Fatalf("first MarkFinished() error = %v", err)
	}

	secondOutput := &domain.RunOutput{
		Stdout:          "hello again",
		Stderr:          "",
		StdoutTruncated: true,
		StderrTruncated: false,
		UpdatedAt:       finishedAt.Add(1 * time.Minute),
	}
	if err := runStore.MarkFinished(ctx, FinishRunParams{
		RunID:      run.ID,
		Status:     domain.RunStatusSucceeded,
		FinishedAt: finishedAt.Add(1 * time.Minute),
		ExitCode:   &exitCode,
		Output:     secondOutput,
	}); err != nil {
		t.Fatalf("second MarkFinished() error = %v", err)
	}

	got, err := runStore.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Status != domain.RunStatusSucceeded {
		t.Fatalf("Status = %q, want %q", got.Status, domain.RunStatusSucceeded)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(startedAt) {
		t.Fatalf("StartedAt = %v, want %v", got.StartedAt, startedAt)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt.Add(1*time.Minute)) {
		t.Fatalf("FinishedAt = %v, want %v", got.FinishedAt, finishedAt.Add(1*time.Minute))
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Fatalf("ExitCode = %v, want 0", got.ExitCode)
	}
	if got.Output == nil {
		t.Fatal("Output = nil, want populated output")
	}
	if got.Output.Stdout != secondOutput.Stdout || got.Output.Stderr != secondOutput.Stderr {
		t.Fatalf("Output text = %#v, want %#v", got.Output, secondOutput)
	}
	if got.Output.StdoutTruncated != secondOutput.StdoutTruncated || got.Output.StderrTruncated != secondOutput.StderrTruncated {
		t.Fatalf("Output truncation = %#v, want %#v", got.Output, secondOutput)
	}
	if !got.Output.UpdatedAt.Equal(secondOutput.UpdatedAt) {
		t.Fatalf("Output UpdatedAt = %v, want %v", got.Output.UpdatedAt, secondOutput.UpdatedAt)
	}
}

func TestRunStoreListFiltersAndGet(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	jobStore := NewJobStore(db)
	runStore := NewRunStore(db)

	cleanup, err := jobStore.Create(ctx, testJob("cleanup"))
	if err != nil {
		t.Fatalf("cleanup Create() error = %v", err)
	}
	archive, err := jobStore.Create(ctx, testJob("archive"))
	if err != nil {
		t.Fatalf("archive Create() error = %v", err)
	}

	cleanupRun, err := runStore.EnqueueManual(ctx, cleanup.ID, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("cleanup EnqueueManual() error = %v", err)
	}
	archiveRun, err := runStore.EnqueueManual(ctx, archive.ID, time.Date(2025, 4, 10, 10, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("archive EnqueueManual() error = %v", err)
	}
	failedRun, err := runStore.EnqueueManual(ctx, cleanup.ID, time.Date(2025, 4, 10, 10, 2, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("failed EnqueueManual() error = %v", err)
	}

	if err := runStore.MarkRunning(ctx, archiveRun.ID, time.Date(2025, 4, 10, 10, 3, 0, 0, time.UTC)); err != nil {
		t.Fatalf("archive MarkRunning() error = %v", err)
	}
	if err := runStore.MarkFinished(ctx, FinishRunParams{
		RunID:      archiveRun.ID,
		Status:     domain.RunStatusSucceeded,
		FinishedAt: time.Date(2025, 4, 10, 10, 4, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("archive MarkFinished() error = %v", err)
	}
	if err := runStore.MarkFinished(ctx, FinishRunParams{
		RunID:      failedRun.ID,
		Status:     domain.RunStatusFailed,
		FinishedAt: time.Date(2025, 4, 10, 10, 5, 0, 0, time.UTC),
		Output: &domain.RunOutput{
			Stdout:    "",
			Stderr:    "boom",
			UpdatedAt: time.Date(2025, 4, 10, 10, 5, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("failed MarkFinished() error = %v", err)
	}

	tests := []struct {
		name   string
		filter ListRunsFilter
		want   []int64
	}{
		{
			name: "all runs ordered by newest queue time",
			want: []int64{failedRun.ID, archiveRun.ID, cleanupRun.ID},
		},
		{
			name: "filter by job name",
			filter: ListRunsFilter{
				JobName: cleanup.Name,
			},
			want: []int64{failedRun.ID, cleanupRun.ID},
		},
		{
			name: "filter by status",
			filter: ListRunsFilter{
				Status: ptrRunStatus(domain.RunStatusSucceeded),
			},
			want: []int64{archiveRun.ID},
		},
		{
			name: "limit results",
			filter: ListRunsFilter{
				Limit: 2,
			},
			want: []int64{failedRun.ID, archiveRun.ID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runs, err := runStore.List(ctx, tt.filter)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(runs) != len(tt.want) {
				t.Fatalf("List() length = %d, want %d", len(runs), len(tt.want))
			}
			for i, wantID := range tt.want {
				if runs[i].ID != wantID {
					t.Fatalf("List()[%d].ID = %d, want %d", i, runs[i].ID, wantID)
				}
			}
		})
	}

	gotCleanup, err := runStore.Get(ctx, cleanupRun.ID)
	if err != nil {
		t.Fatalf("Get(cleanupRun) error = %v", err)
	}
	if gotCleanup.Output != nil {
		t.Fatalf("Get(cleanupRun).Output = %#v, want nil", gotCleanup.Output)
	}

	gotFailed, err := runStore.Get(ctx, failedRun.ID)
	if err != nil {
		t.Fatalf("Get(failedRun) error = %v", err)
	}
	if gotFailed.Output == nil || gotFailed.Output.Stderr != "boom" {
		t.Fatalf("Get(failedRun).Output = %#v, want stderr boom", gotFailed.Output)
	}
}

func TestRunStoreCancelPendingByJob(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	jobStore := NewJobStore(db)
	runStore := NewRunStore(db)

	targetJob, err := jobStore.Create(ctx, testJob("cleanup"))
	if err != nil {
		t.Fatalf("target Create() error = %v", err)
	}
	otherJob, err := jobStore.Create(ctx, testJob("archive"))
	if err != nil {
		t.Fatalf("other Create() error = %v", err)
	}

	pendingOne, err := runStore.EnqueueManual(ctx, targetJob.ID, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("pendingOne EnqueueManual() error = %v", err)
	}
	pendingTwo, err := runStore.EnqueueManual(ctx, targetJob.ID, time.Date(2025, 4, 10, 10, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("pendingTwo EnqueueManual() error = %v", err)
	}
	running, err := runStore.EnqueueManual(ctx, targetJob.ID, time.Date(2025, 4, 10, 10, 2, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("running EnqueueManual() error = %v", err)
	}
	otherPending, err := runStore.EnqueueManual(ctx, otherJob.ID, time.Date(2025, 4, 10, 10, 3, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("otherPending EnqueueManual() error = %v", err)
	}

	if err := runStore.MarkRunning(ctx, running.ID, time.Date(2025, 4, 10, 10, 4, 0, 0, time.UTC)); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}

	canceledAt := time.Date(2025, 4, 10, 10, 5, 0, 0, time.UTC)
	if err := runStore.CancelPendingByJob(ctx, targetJob.ID, canceledAt); err != nil {
		t.Fatalf("CancelPendingByJob() error = %v", err)
	}

	for _, runID := range []int64{pendingOne.ID, pendingTwo.ID} {
		run, err := runStore.Get(ctx, runID)
		if err != nil {
			t.Fatalf("Get(%d) error = %v", runID, err)
		}
		if run.Status != domain.RunStatusCanceled {
			t.Fatalf("Get(%d).Status = %q, want %q", runID, run.Status, domain.RunStatusCanceled)
		}
		if run.FinishedAt == nil || !run.FinishedAt.Equal(canceledAt) {
			t.Fatalf("Get(%d).FinishedAt = %v, want %v", runID, run.FinishedAt, canceledAt)
		}
	}

	stillRunning, err := runStore.Get(ctx, running.ID)
	if err != nil {
		t.Fatalf("Get(running) error = %v", err)
	}
	if stillRunning.Status != domain.RunStatusRunning {
		t.Fatalf("running status = %q, want %q", stillRunning.Status, domain.RunStatusRunning)
	}

	stillPending, err := runStore.Get(ctx, otherPending.ID)
	if err != nil {
		t.Fatalf("Get(otherPending) error = %v", err)
	}
	if stillPending.Status != domain.RunStatusPending {
		t.Fatalf("otherPending status = %q, want %q", stillPending.Status, domain.RunStatusPending)
	}
}

func TestRunStoreCascadeDelete(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	jobStore := NewJobStore(db)
	runStore := NewRunStore(db)

	job, err := jobStore.Create(ctx, testJob("cleanup"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	run, err := runStore.EnqueueManual(ctx, job.ID, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EnqueueManual() error = %v", err)
	}
	if err := runStore.MarkFinished(ctx, FinishRunParams{
		RunID:      run.ID,
		Status:     domain.RunStatusSucceeded,
		FinishedAt: time.Date(2025, 4, 10, 10, 1, 0, 0, time.UTC),
		Output: &domain.RunOutput{
			Stdout:    "done",
			UpdatedAt: time.Date(2025, 4, 10, 10, 1, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("MarkFinished() error = %v", err)
	}

	if err := jobStore.DeleteByName(ctx, job.Name); err != nil {
		t.Fatalf("DeleteByName() error = %v", err)
	}

	if _, err := runStore.Get(ctx, run.ID); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("Get() after delete error = %v, want %v", err, ErrRunNotFound)
	}

	assertCount(t, db, "SELECT COUNT(*) FROM job_runs", 0)
	assertCount(t, db, "SELECT COUNT(*) FROM job_run_outputs", 0)
}

func assertRunCore(t *testing.T, got domain.Run, want domain.Run) {
	t.Helper()

	if got.ID != want.ID {
		t.Fatalf("ID = %d, want %d", got.ID, want.ID)
	}
	if got.JobID != want.JobID {
		t.Fatalf("JobID = %d, want %d", got.JobID, want.JobID)
	}
	if got.JobName != want.JobName {
		t.Fatalf("JobName = %q, want %q", got.JobName, want.JobName)
	}
	if got.TriggerType != want.TriggerType {
		t.Fatalf("TriggerType = %q, want %q", got.TriggerType, want.TriggerType)
	}
	if got.Status != want.Status {
		t.Fatalf("Status = %q, want %q", got.Status, want.Status)
	}
	assertTimePtrEqual(t, "ScheduledFor", got.ScheduledFor, want.ScheduledFor)
	if !got.QueuedAt.Equal(want.QueuedAt) {
		t.Fatalf("QueuedAt = %v, want %v", got.QueuedAt, want.QueuedAt)
	}
}

func ptrRunStatus(status domain.RunStatus) *domain.RunStatus {
	return &status
}

func assertCount(t *testing.T, db queryCounter, query string, want int) {
	t.Helper()

	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("QueryRow(%q) error = %v", query, err)
	}
	if got != want {
		t.Fatalf("count for %q = %d, want %d", query, got, want)
	}
}

type queryCounter interface {
	QueryRow(query string, args ...any) *sql.Row
}
