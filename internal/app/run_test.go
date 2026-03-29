package app

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/sqlite"
)

func TestRunCommandsRequireFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "list missing instance",
			args: []string{"run", "list"},
			want: `required flag(s) "instance" not set`,
		},
		{
			name: "get missing instance",
			args: []string{"run", "get", "--run-id", "1"},
			want: `required flag(s) "instance" not set`,
		},
		{
			name: "get missing run id",
			args: []string{"run", "get", "--instance", "dev"},
			want: `required flag(s) "run-id" not set`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTestDirs(t)

			_, err := executeRootCommand(t, tt.args...)
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if err.Error() != tt.want {
				t.Fatalf("Execute() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestRunCommandValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "invalid status",
			args: []string{"run", "list", "--instance", "dev", "--status", "done"},
			want: `invalid run status`,
		},
		{
			name: "invalid limit",
			args: []string{"run", "list", "--instance", "dev", "--limit", "0"},
			want: `limit must be greater than zero`,
		},
		{
			name: "invalid run id",
			args: []string{"run", "get", "--instance", "dev", "--run-id", "0"},
			want: `run-id must be greater than zero`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTestDirs(t)

			_, err := executeRootCommand(t, tt.args...)
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Execute() error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestRunCommandsIntegration(t *testing.T) {
	setTestDirs(t)

	db, cleanup := openInstanceDBForTest(t, "dev")
	defer cleanup()

	ctx := context.Background()
	jobStore := db.Jobs
	runStore := db.Runs

	base := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	cleanupJob, err := jobStore.Create(ctx, domain.Job{
		Name:              "cleanup",
		Command:           "echo cleanup",
		ScheduleKind:      domain.ScheduleKindInterval,
		ScheduleExpr:      "every 10m",
		Timezone:          "UTC",
		Enabled:           true,
		ConcurrencyPolicy: domain.ConcurrencyPolicyQueue,
		CreatedAt:         base,
		UpdatedAt:         base,
	})
	if err != nil {
		t.Fatalf("cleanup Create() error = %v", err)
	}
	archiveJob, err := jobStore.Create(ctx, domain.Job{
		Name:              "archive",
		Command:           "echo archive",
		ScheduleKind:      domain.ScheduleKindInterval,
		ScheduleExpr:      "every 30m",
		Timezone:          "UTC",
		Enabled:           true,
		ConcurrencyPolicy: domain.ConcurrencyPolicyQueue,
		CreatedAt:         base,
		UpdatedAt:         base,
	})
	if err != nil {
		t.Fatalf("archive Create() error = %v", err)
	}

	cleanupRun, err := runStore.EnqueueManual(ctx, cleanupJob.ID, base)
	if err != nil {
		t.Fatalf("cleanup EnqueueManual() error = %v", err)
	}
	archiveRun, err := runStore.EnqueueManual(ctx, archiveJob.ID, base.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("archive EnqueueManual() error = %v", err)
	}
	failedRun, err := runStore.EnqueueManual(ctx, cleanupJob.ID, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("failed EnqueueManual() error = %v", err)
	}

	if err := runStore.MarkRunning(ctx, archiveRun.ID, base.Add(3*time.Minute)); err != nil {
		t.Fatalf("archive MarkRunning() error = %v", err)
	}
	if err := runStore.MarkFinished(ctx, sqlite.FinishRunParams{
		RunID:      archiveRun.ID,
		Status:     domain.RunStatusSucceeded,
		FinishedAt: base.Add(4 * time.Minute),
	}); err != nil {
		t.Fatalf("archive MarkFinished() error = %v", err)
	}
	if err := runStore.MarkRunning(ctx, failedRun.ID, base.Add(5*time.Minute)); err != nil {
		t.Fatalf("failed MarkRunning() error = %v", err)
	}
	exitCode := 1
	errorMessage := "command failed"
	if err := runStore.MarkFinished(ctx, sqlite.FinishRunParams{
		RunID:        failedRun.ID,
		Status:       domain.RunStatusFailed,
		FinishedAt:   base.Add(6 * time.Minute),
		ExitCode:     &exitCode,
		ErrorMessage: &errorMessage,
		Output: &domain.RunOutput{
			Stdout:          strings.Repeat("a", 64),
			Stderr:          "boom\nsecond line",
			StdoutTruncated: true,
			StderrTruncated: false,
			UpdatedAt:       base.Add(6 * time.Minute),
		},
	}); err != nil {
		t.Fatalf("failed MarkFinished() error = %v", err)
	}

	stdout, err := executeRootCommand(t, "--output", "json", "run", "list", "--instance", "dev", "--job", "cleanup", "--status", "failed", "--limit", "1")
	if err != nil {
		t.Fatalf("run list error = %v", err)
	}

	var listed []runSummaryOutput
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil {
		t.Fatalf("Unmarshal(run list) error = %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("len(listed) = %d, want 1", len(listed))
	}
	if listed[0].ID != failedRun.ID {
		t.Fatalf("listed[0].ID = %d, want %d", listed[0].ID, failedRun.ID)
	}

	stdout, err = executeRootCommand(t, "--output", "json", "run", "get", "--instance", "dev", "--run-id", "9999")
	if err == nil {
		t.Fatal("run get missing error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run 9999 not found") {
		t.Fatalf("run get missing error = %q, want not found", err.Error())
	}

	stdout, err = executeRootCommand(t, "--output", "json", "run", "get", "--instance", "dev", "--run-id", int64ToString(failedRun.ID))
	if err != nil {
		t.Fatalf("run get error = %v", err)
	}

	var detail runDetailOutput
	if err := json.Unmarshal([]byte(stdout), &detail); err != nil {
		t.Fatalf("Unmarshal(run get) error = %v", err)
	}
	if detail.ID != failedRun.ID {
		t.Fatalf("detail.ID = %d, want %d", detail.ID, failedRun.ID)
	}
	if !detail.StdoutTruncated || detail.StderrTruncated {
		t.Fatalf("detail truncation = stdout:%t stderr:%t, want true/false", detail.StdoutTruncated, detail.StderrTruncated)
	}
	if detail.Output == nil || detail.Output.Stderr != "boom\nsecond line" {
		t.Fatalf("detail.Output = %#v, want populated stderr", detail.Output)
	}
	if detail.StdoutPreview == "" || detail.StderrPreview == "" {
		t.Fatalf("preview fields should not be empty: %#v", detail)
	}
	if detail.Duration != "1m0s" {
		t.Fatalf("detail.Duration = %q, want 1m0s", detail.Duration)
	}

	stdout, err = executeRootCommand(t, "--output", "json", "run", "list", "--instance", "dev")
	if err != nil {
		t.Fatalf("run list all error = %v", err)
	}

	listed = nil
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil {
		t.Fatalf("Unmarshal(run list all) error = %v", err)
	}
	if len(listed) != 3 {
		t.Fatalf("len(listed all) = %d, want 3", len(listed))
	}
	if listed[0].ID != failedRun.ID || listed[1].ID != archiveRun.ID || listed[2].ID != cleanupRun.ID {
		t.Fatalf("listed order = %#v, want [%d %d %d]", listed, failedRun.ID, archiveRun.ID, cleanupRun.ID)
	}
}

func int64ToString(value int64) string {
	return strconv.FormatInt(value, 10)
}
