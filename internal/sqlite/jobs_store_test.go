package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestJobStoreCRUD(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	store := NewJobStore(db)

	createdAt := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	nextRunAt := createdAt.Add(30 * time.Minute)
	job := domain.Job{
		Name:              "cleanup",
		Command:           "echo cleanup",
		ScheduleKind:      domain.ScheduleKindInterval,
		ScheduleExpr:      "every 30m",
		Timezone:          "UTC",
		Enabled:           true,
		ConcurrencyPolicy: domain.ConcurrencyPolicyQueue,
		NextRunAt:         &nextRunAt,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}

	created, err := store.Create(ctx, job)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.ID == 0 {
		t.Fatal("Create() returned empty job ID")
	}
	assertJobMatches(t, created, domain.Job{
		ID:                created.ID,
		Name:              job.Name,
		Command:           job.Command,
		ScheduleKind:      job.ScheduleKind,
		ScheduleExpr:      job.ScheduleExpr,
		Timezone:          job.Timezone,
		Enabled:           job.Enabled,
		ConcurrencyPolicy: job.ConcurrencyPolicy,
		NextRunAt:         job.NextRunAt,
		CreatedAt:         job.CreatedAt,
		UpdatedAt:         job.UpdatedAt,
	})

	gotByName, err := store.GetByName(ctx, created.Name)
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	assertJobMatches(t, gotByName, created)

	gotByID, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	assertJobMatches(t, gotByID, created)

	second, err := store.Create(ctx, domain.Job{
		Name:              "archive",
		Command:           "echo archive",
		ScheduleKind:      domain.ScheduleKindCron,
		ScheduleExpr:      "cron */5 * * * *",
		Timezone:          "Local",
		Enabled:           false,
		ConcurrencyPolicy: domain.ConcurrencyPolicyForbid,
		CreatedAt:         createdAt.Add(1 * time.Hour),
		UpdatedAt:         createdAt.Add(1*time.Hour + 5*time.Minute),
	})
	if err != nil {
		t.Fatalf("second Create() error = %v", err)
	}

	jobs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("List() length = %d, want 2", len(jobs))
	}
	if jobs[0].Name != "archive" || jobs[1].Name != "cleanup" {
		t.Fatalf("List() order = [%s %s], want [archive cleanup]", jobs[0].Name, jobs[1].Name)
	}

	lastRunStatus := domain.RunStatusSucceeded
	lastRunAt := createdAt.Add(2 * time.Hour)
	updatedJob, err := store.Update(ctx, domain.Job{
		ID:                created.ID,
		Name:              "cleanup-nightly",
		Command:           "echo cleanup-nightly",
		ScheduleKind:      domain.ScheduleKindOnce,
		ScheduleExpr:      "after 10m",
		Timezone:          "Asia/Seoul",
		Enabled:           false,
		ConcurrencyPolicy: domain.ConcurrencyPolicyReplace,
		LastRunAt:         &lastRunAt,
		LastRunStatus:     &lastRunStatus,
		CreatedAt:         created.CreatedAt,
		UpdatedAt:         created.UpdatedAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	assertJobMatches(t, updatedJob, domain.Job{
		ID:                created.ID,
		Name:              "cleanup-nightly",
		Command:           "echo cleanup-nightly",
		ScheduleKind:      domain.ScheduleKindOnce,
		ScheduleExpr:      "after 10m",
		Timezone:          "Asia/Seoul",
		Enabled:           false,
		ConcurrencyPolicy: domain.ConcurrencyPolicyReplace,
		LastRunAt:         &lastRunAt,
		LastRunStatus:     &lastRunStatus,
		CreatedAt:         created.CreatedAt,
		UpdatedAt:         created.UpdatedAt.Add(2 * time.Hour),
	})

	if err := store.DeleteByName(ctx, second.Name); err != nil {
		t.Fatalf("DeleteByName() error = %v", err)
	}

	if _, err := store.GetByName(ctx, second.Name); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("GetByName() after delete error = %v, want %v", err, ErrJobNotFound)
	}
}

func TestJobStoreDeleteByNameNotFound(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	store := NewJobStore(db)

	err := store.DeleteByName(ctx, "missing")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("DeleteByName() error = %v, want %v", err, ErrJobNotFound)
	}
}

func TestJobStoreUniqueName(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	store := NewJobStore(db)

	job := testJob("cleanup")
	if _, err := store.Create(ctx, job); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if _, err := store.Create(ctx, job); err == nil {
		t.Fatal("second Create() error = nil, want unique constraint error")
	}

	jobs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("List() length = %d, want 1", len(jobs))
	}
}

func TestJobStoreListDue(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	store := NewJobStore(db)

	base := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	jobs := []domain.Job{
		testJobWithNextRun("due-first", base.Add(-2*time.Minute), true),
		testJobWithNextRun("not-due", base.Add(5*time.Minute), true),
		testJobWithNextRun("due-second", base.Add(-1*time.Minute), true),
		testJobWithNextRun("disabled", base.Add(-3*time.Minute), false),
		testJob("no-next-run"),
	}

	for _, job := range jobs {
		if _, err := store.Create(ctx, job); err != nil {
			t.Fatalf("Create(%s) error = %v", job.Name, err)
		}
	}

	due, err := store.ListDue(ctx, base)
	if err != nil {
		t.Fatalf("ListDue() error = %v", err)
	}

	if len(due) != 2 {
		t.Fatalf("ListDue() length = %d, want 2", len(due))
	}
	if due[0].Name != "due-first" || due[1].Name != "due-second" {
		t.Fatalf("ListDue() order = [%s %s], want [due-first due-second]", due[0].Name, due[1].Name)
	}
}

func TestJobStoreUpdateNextRunAndLastRunSummary(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	store := NewJobStore(db)

	created, err := store.Create(ctx, testJobWithNextRun("cleanup", time.Date(2025, 4, 10, 10, 30, 0, 0, time.UTC), true))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	nextUpdatedAt := time.Date(2025, 4, 10, 11, 0, 0, 0, time.UTC)
	if err := store.UpdateNextRun(ctx, created.ID, nil, nextUpdatedAt); err != nil {
		t.Fatalf("UpdateNextRun() error = %v", err)
	}

	got, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() after UpdateNextRun error = %v", err)
	}
	if got.NextRunAt != nil {
		t.Fatalf("NextRunAt = %v, want nil", *got.NextRunAt)
	}
	if !got.UpdatedAt.Equal(nextUpdatedAt) {
		t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, nextUpdatedAt)
	}

	lastRunAt := time.Date(2025, 4, 10, 12, 0, 0, 0, time.UTC)
	lastRunStatus := domain.RunStatusFailed
	lastUpdatedAt := time.Date(2025, 4, 10, 12, 5, 0, 0, time.UTC)
	if err := store.UpdateLastRunSummary(ctx, created.ID, &lastRunAt, &lastRunStatus, lastUpdatedAt); err != nil {
		t.Fatalf("UpdateLastRunSummary() error = %v", err)
	}

	got, err = store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() after UpdateLastRunSummary error = %v", err)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(lastRunAt) {
		t.Fatalf("LastRunAt = %v, want %v", got.LastRunAt, lastRunAt)
	}
	if got.LastRunStatus == nil || *got.LastRunStatus != lastRunStatus {
		t.Fatalf("LastRunStatus = %v, want %v", got.LastRunStatus, lastRunStatus)
	}
	if !got.UpdatedAt.Equal(lastUpdatedAt) {
		t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, lastUpdatedAt)
	}
}

func testJob(name string) domain.Job {
	base := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)

	return domain.Job{
		Name:              name,
		Command:           "echo " + name,
		ScheduleKind:      domain.ScheduleKindInterval,
		ScheduleExpr:      "every 10m",
		Timezone:          "UTC",
		Enabled:           true,
		ConcurrencyPolicy: domain.ConcurrencyPolicyForbid,
		CreatedAt:         base,
		UpdatedAt:         base.Add(1 * time.Minute),
	}
}

func testJobWithNextRun(name string, nextRunAt time.Time, enabled bool) domain.Job {
	job := testJob(name)
	job.NextRunAt = &nextRunAt
	job.Enabled = enabled
	return job
}

func assertJobMatches(t *testing.T, got domain.Job, want domain.Job) {
	t.Helper()

	if got.ID != want.ID {
		t.Fatalf("ID = %d, want %d", got.ID, want.ID)
	}
	if got.Name != want.Name {
		t.Fatalf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.Command != want.Command {
		t.Fatalf("Command = %q, want %q", got.Command, want.Command)
	}
	if got.ScheduleKind != want.ScheduleKind {
		t.Fatalf("ScheduleKind = %q, want %q", got.ScheduleKind, want.ScheduleKind)
	}
	if got.ScheduleExpr != want.ScheduleExpr {
		t.Fatalf("ScheduleExpr = %q, want %q", got.ScheduleExpr, want.ScheduleExpr)
	}
	if got.Timezone != want.Timezone {
		t.Fatalf("Timezone = %q, want %q", got.Timezone, want.Timezone)
	}
	if got.Enabled != want.Enabled {
		t.Fatalf("Enabled = %v, want %v", got.Enabled, want.Enabled)
	}
	if got.ConcurrencyPolicy != want.ConcurrencyPolicy {
		t.Fatalf("ConcurrencyPolicy = %q, want %q", got.ConcurrencyPolicy, want.ConcurrencyPolicy)
	}
	assertTimePtrEqual(t, "NextRunAt", got.NextRunAt, want.NextRunAt)
	assertTimePtrEqual(t, "LastRunAt", got.LastRunAt, want.LastRunAt)
	assertRunStatusPtrEqual(t, got.LastRunStatus, want.LastRunStatus)
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, want.UpdatedAt)
	}
}

func assertTimePtrEqual(t *testing.T, field string, got *time.Time, want *time.Time) {
	t.Helper()

	if got == nil || want == nil {
		if got != nil || want != nil {
			t.Fatalf("%s = %v, want %v", field, got, want)
		}
		return
	}
	if !got.Equal(*want) {
		t.Fatalf("%s = %v, want %v", field, *got, *want)
	}
}

func assertRunStatusPtrEqual(t *testing.T, got *domain.RunStatus, want *domain.RunStatus) {
	t.Helper()

	if got == nil || want == nil {
		if got != nil || want != nil {
			t.Fatalf("LastRunStatus = %v, want %v", got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("LastRunStatus = %q, want %q", *got, *want)
	}
}
