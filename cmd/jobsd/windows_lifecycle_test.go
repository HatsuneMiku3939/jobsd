//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestWindowsSchedulerLifecycle(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(baseDir, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(baseDir, "runtime"))

	binaryPath := buildJobsdBinary(t)
	firstInstance := "win-start-stop"
	secondInstance := "win-second"
	firstPort := freePort(t)
	secondPort := freePort(t)

	t.Cleanup(func() {
		runStopIfRunning(t, binaryPath, firstInstance)
		runStopIfRunning(t, binaryPath, secondInstance)
	})

	startOut := runJobsdJSON[schedulerCommandOutput](t, binaryPath, "scheduler", "start", "--instance", firstInstance, "--port", fmt.Sprintf("%d", firstPort))
	if startOut.Status != domain.SchedulerStatusRunning {
		t.Fatalf("start status = %q, want %q", startOut.Status, domain.SchedulerStatusRunning)
	}

	firstPaths, err := config.ResolvePaths(firstInstance)
	if err != nil {
		t.Fatalf("ResolvePaths() first error = %v", err)
	}
	waitForStateFile(t, firstPaths.StatePath, 5*time.Second)

	statusOut := runJobsdJSON[schedulerCommandOutput](t, binaryPath, "scheduler", "status", "--instance", firstInstance)
	if statusOut.Status != domain.SchedulerStatusRunning {
		t.Fatalf("status = %q, want %q", statusOut.Status, domain.SchedulerStatusRunning)
	}

	pingOut := runJobsdJSON[schedulerCommandOutput](t, binaryPath, "scheduler", "ping", "--instance", firstInstance)
	if pingOut.Status != domain.SchedulerStatusRunning {
		t.Fatalf("ping status = %q, want %q", pingOut.Status, domain.SchedulerStatusRunning)
	}

	err = runJobsdExpectError(binaryPath, "scheduler", "start", "--instance", firstInstance, "--port", fmt.Sprintf("%d", freePort(t)))
	if err == nil {
		t.Fatal("duplicate start error = nil, want error")
	}
	if got := err.Error(); got != fmt.Sprintf("instance %q is already running", firstInstance) {
		t.Fatalf("duplicate start error = %q, want already running error", got)
	}

	secondStartOut := runJobsdJSON(t, binaryPath, "scheduler", "start", "--instance", secondInstance, "--port", fmt.Sprintf("%d", secondPort))
	if secondStartOut.Status != domain.SchedulerStatusRunning {
		t.Fatalf("second start status = %q, want %q", secondStartOut.Status, domain.SchedulerStatusRunning)
	}

	secondPaths, err := config.ResolvePaths(secondInstance)
	if err != nil {
		t.Fatalf("ResolvePaths() second error = %v", err)
	}
	waitForStateFile(t, secondPaths.StatePath, 5*time.Second)

	secondStatusOut := runJobsdJSON[schedulerCommandOutput](t, binaryPath, "scheduler", "status", "--instance", secondInstance)
	if secondStatusOut.Status != domain.SchedulerStatusRunning {
		t.Fatalf("second status = %q, want %q", secondStatusOut.Status, domain.SchedulerStatusRunning)
	}

	stopOut := runJobsdJSON[schedulerCommandOutput](t, binaryPath, "scheduler", "stop", "--instance", firstInstance)
	if stopOut.Status != domain.SchedulerStatusStopped {
		t.Fatalf("stop status = %q, want %q", stopOut.Status, domain.SchedulerStatusStopped)
	}
	waitForStateRemoval(t, firstPaths.StatePath, 5*time.Second)

	stoppedOut := runJobsdJSON[schedulerCommandOutput](t, binaryPath, "scheduler", "status", "--instance", firstInstance)
	if stoppedOut.Status != domain.SchedulerStatusStopped {
		t.Fatalf("post-stop status = %q, want %q", stoppedOut.Status, domain.SchedulerStatusStopped)
	}
}
