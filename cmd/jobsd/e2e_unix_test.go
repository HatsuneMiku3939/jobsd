//go:build e2e && !windows

package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestE2EManualWorkflow(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(baseDir, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(baseDir, "runtime"))

	binaryPath := buildJobsdBinary(t)
	instance := "e2e-manual"
	jobName := "manual-workflow"

	t.Cleanup(func() {
		runStopIfRunning(t, binaryPath, instance)
	})

	startOut := runJobsdJSON[schedulerCommandOutput](t, binaryPath, "scheduler", "start", "--instance", instance)
	if startOut.Status != domain.SchedulerStatusRunning {
		t.Fatalf("start status = %q, want %q", startOut.Status, domain.SchedulerStatusRunning)
	}

	paths, err := config.ResolvePaths(instance)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	waitForStateFile(t, paths.StatePath, 5*time.Second)

	added := runJobsdJSON[jobCommandOutput](
		t,
		binaryPath,
		"job", "add",
		"--instance", instance,
		"--name", jobName,
		"--schedule", "every 1h",
		"--timezone", "UTC",
		"--command", "printf 'manual-e2e'",
	)
	if added.Name != jobName {
		t.Fatalf("added.Name = %q, want %q", added.Name, jobName)
	}

	enqueued := runJobsdJSON[runEnqueueOutput](t, binaryPath, "job", "run", "--instance", instance, "--name", jobName)
	if enqueued.RunID == 0 {
		t.Fatal("enqueued.RunID = 0, want non-zero")
	}
	if enqueued.TriggerType != string(domain.RunTriggerTypeManual) {
		t.Fatalf("enqueued.TriggerType = %q, want %q", enqueued.TriggerType, domain.RunTriggerTypeManual)
	}

	detail := waitForRunStatus(t, binaryPath, instance, enqueued.RunID, 15*time.Second, domain.RunStatusSucceeded)
	if detail.TriggerType != string(domain.RunTriggerTypeManual) {
		t.Fatalf("detail.TriggerType = %q, want %q", detail.TriggerType, domain.RunTriggerTypeManual)
	}
	if detail.Status != string(domain.RunStatusSucceeded) {
		t.Fatalf("detail.Status = %q, want %q", detail.Status, domain.RunStatusSucceeded)
	}
	if detail.ExitCode == nil || *detail.ExitCode != 0 {
		t.Fatalf("detail.ExitCode = %v, want 0", detail.ExitCode)
	}
	if detail.Output == nil {
		t.Fatal("detail.Output = nil, want captured output")
	}
	if detail.Output.Stdout != "manual-e2e" {
		t.Fatalf("detail.Output.Stdout = %q, want %q", detail.Output.Stdout, "manual-e2e")
	}
	if detail.Output.Stderr != "" {
		t.Fatalf("detail.Output.Stderr = %q, want empty", detail.Output.Stderr)
	}
	if detail.StdoutTruncated || detail.StderrTruncated {
		t.Fatalf("detail truncation = stdout:%t stderr:%t, want false/false", detail.StdoutTruncated, detail.StderrTruncated)
	}

	runs := runJobsdJSON[[]runSummaryOutput](t, binaryPath, "run", "list", "--instance", instance, "--job", jobName, "--limit", "1")
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runs[0].ID != enqueued.RunID {
		t.Fatalf("runs[0].ID = %d, want %d", runs[0].ID, enqueued.RunID)
	}

	stopOut := runJobsdJSON[schedulerCommandOutput](t, binaryPath, "scheduler", "stop", "--instance", instance)
	if stopOut.Status != domain.SchedulerStatusStopped {
		t.Fatalf("stop status = %q, want %q", stopOut.Status, domain.SchedulerStatusStopped)
	}
	waitForStateRemoval(t, paths.StatePath, 5*time.Second)

	statusOut := runJobsdJSON[schedulerCommandOutput](t, binaryPath, "scheduler", "status", "--instance", instance)
	if statusOut.Status != domain.SchedulerStatusStopped {
		t.Fatalf("status after stop = %q, want %q", statusOut.Status, domain.SchedulerStatusStopped)
	}
}

func TestE2EAutomaticIntervalExecution(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(baseDir, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(baseDir, "runtime"))

	binaryPath := buildJobsdBinary(t)
	instance := "e2e-interval"
	jobName := "interval-job"

	t.Cleanup(func() {
		runStopIfRunning(t, binaryPath, instance)
	})

	startSchedulerForE2E(t, binaryPath, instance)

	runJobsdJSON[jobCommandOutput](
		t,
		binaryPath,
		"job", "add",
		"--instance", instance,
		"--name", jobName,
		"--schedule", "every 2s",
		"--timezone", "UTC",
		"--command", "printf 'interval-e2e'",
	)

	run := waitForLatestRun(t, binaryPath, instance, jobName, 15*time.Second, func(run runSummaryOutput) bool {
		return run.Trigger == string(domain.RunTriggerTypeSchedule) && run.Status == string(domain.RunStatusSucceeded)
	})

	detail := waitForRunStatus(t, binaryPath, instance, run.ID, 5*time.Second, domain.RunStatusSucceeded)
	if detail.TriggerType != string(domain.RunTriggerTypeSchedule) {
		t.Fatalf("detail.TriggerType = %q, want %q", detail.TriggerType, domain.RunTriggerTypeSchedule)
	}
	if detail.ExitCode == nil || *detail.ExitCode != 0 {
		t.Fatalf("detail.ExitCode = %v, want 0", detail.ExitCode)
	}
	if detail.Output == nil || detail.Output.Stdout != "interval-e2e" {
		t.Fatalf("detail.Output = %#v, want stdout interval-e2e", detail.Output)
	}
}

func TestE2EAutomaticCronExecution(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(baseDir, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(baseDir, "runtime"))

	binaryPath := buildJobsdBinary(t)
	instance := "e2e-cron"
	jobName := "cron-job"

	t.Cleanup(func() {
		runStopIfRunning(t, binaryPath, instance)
	})

	startSchedulerForE2E(t, binaryPath, instance)

	runJobsdJSON[jobCommandOutput](
		t,
		binaryPath,
		"job", "add",
		"--instance", instance,
		"--name", jobName,
		"--schedule", "cron * * * * *",
		"--timezone", "UTC",
		"--command", "printf 'cron-e2e'",
	)

	run := waitForLatestRun(t, binaryPath, instance, jobName, 75*time.Second, func(run runSummaryOutput) bool {
		return run.Trigger == string(domain.RunTriggerTypeSchedule) && run.Status == string(domain.RunStatusSucceeded)
	})

	detail := waitForRunStatus(t, binaryPath, instance, run.ID, 5*time.Second, domain.RunStatusSucceeded)
	if detail.TriggerType != string(domain.RunTriggerTypeSchedule) {
		t.Fatalf("detail.TriggerType = %q, want %q", detail.TriggerType, domain.RunTriggerTypeSchedule)
	}
	if detail.ExitCode == nil || *detail.ExitCode != 0 {
		t.Fatalf("detail.ExitCode = %v, want 0", detail.ExitCode)
	}
	if detail.Output == nil || detail.Output.Stdout != "cron-e2e" {
		t.Fatalf("detail.Output = %#v, want stdout cron-e2e", detail.Output)
	}
}

func TestE2EOneTimeExecutionDisablesJob(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(baseDir, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(baseDir, "runtime"))

	binaryPath := buildJobsdBinary(t)
	instance := "e2e-once"
	jobName := "once-job"

	t.Cleanup(func() {
		runStopIfRunning(t, binaryPath, instance)
	})

	startSchedulerForE2E(t, binaryPath, instance)

	runJobsdJSON[jobCommandOutput](
		t,
		binaryPath,
		"job", "add",
		"--instance", instance,
		"--name", jobName,
		"--schedule", "after 2s",
		"--timezone", "UTC",
		"--command", "printf 'once-e2e'",
	)

	run := waitForLatestRun(t, binaryPath, instance, jobName, 15*time.Second, func(run runSummaryOutput) bool {
		return run.Trigger == string(domain.RunTriggerTypeSchedule) && run.Status == string(domain.RunStatusSucceeded)
	})

	detail := waitForRunStatus(t, binaryPath, instance, run.ID, 5*time.Second, domain.RunStatusSucceeded)
	if detail.Output == nil || detail.Output.Stdout != "once-e2e" {
		t.Fatalf("detail.Output = %#v, want stdout once-e2e", detail.Output)
	}

	job := runJobsdJSON[jobCommandOutput](t, binaryPath, "job", "get", "--instance", instance, "--name", jobName)
	if job.Enabled {
		t.Fatal("job.Enabled = true, want false")
	}
	if job.NextRunAt != nil {
		t.Fatalf("job.NextRunAt = %v, want nil", job.NextRunAt)
	}
	if job.LastRunStatus == nil || *job.LastRunStatus != string(domain.RunStatusSucceeded) {
		t.Fatalf("job.LastRunStatus = %v, want %q", job.LastRunStatus, domain.RunStatusSucceeded)
	}

	time.Sleep(4 * time.Second)
	runs := runJobsdJSON[[]runSummaryOutput](t, binaryPath, "run", "list", "--instance", instance, "--job", jobName, "--limit", "20")
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
}

func TestE2EDuplicateStartRejected(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(baseDir, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(baseDir, "runtime"))

	binaryPath := buildJobsdBinary(t)
	instance := "e2e-duplicate"

	t.Cleanup(func() {
		runStopIfRunning(t, binaryPath, instance)
	})

	startSchedulerForE2E(t, binaryPath, instance)

	err := runJobsdExpectError(binaryPath, "scheduler", "start", "--instance", instance)
	if err == nil {
		t.Fatal("duplicate start error = nil, want error")
	}
	want := fmt.Sprintf("instance %q is already running", instance)
	if err.Error() != want {
		t.Fatalf("duplicate start error = %q, want %q", err.Error(), want)
	}
}

func startSchedulerForE2E(t *testing.T, binaryPath string, instance string) {
	t.Helper()

	startOut := runJobsdJSON[schedulerCommandOutput](t, binaryPath, "scheduler", "start", "--instance", instance)
	if startOut.Status != domain.SchedulerStatusRunning {
		t.Fatalf("start status = %q, want %q", startOut.Status, domain.SchedulerStatusRunning)
	}

	paths, err := config.ResolvePaths(instance)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	waitForStateFile(t, paths.StatePath, 5*time.Second)
}
