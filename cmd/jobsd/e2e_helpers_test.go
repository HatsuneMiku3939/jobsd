package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/daemon"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

const jobsdCommandTimeout = 30 * time.Second

type schedulerCommandOutput struct {
	Instance  string                 `json:"instance"`
	Status    domain.SchedulerStatus `json:"status"`
	PID       int                    `json:"pid"`
	Port      int                    `json:"port"`
	DBPath    string                 `json:"db_path"`
	StartedAt string                 `json:"started_at"`
	Version   string                 `json:"version"`
	Reason    string                 `json:"reason"`
}

type jobCommandOutput struct {
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

func buildJobsdBinary(t *testing.T) string {
	t.Helper()

	name := "jobsd"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	binaryPath := filepath.Join(t.TempDir(), name)
	cmd, cancel := newTimedCommand(t, "go", "build", "-o", binaryPath, ".")
	defer cancel()
	cmd.Dir = "."
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		failTimedCommand(t, cmd, err, output)
	}

	return binaryPath
}

func runJobsdJSON[T any](t *testing.T, binaryPath string, args ...string) T {
	t.Helper()

	output := runJobsd(t, binaryPath, append([]string{"--output", "json"}, args...)...)

	var payload T
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v, output = %s", err, output)
	}

	return payload
}

func runJobsdExpectError(binaryPath string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), jobsdCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command timed out after %s: %s %v", jobsdCommandTimeout, binaryPath, args)
	}
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(output) > 0 {
		return fmt.Errorf("%s", string(bytesTrimSpace(output)))
	}

	return err
}

func runJobsd(t *testing.T, binaryPath string, args ...string) []byte {
	t.Helper()

	cmd, cancel := newTimedCommand(t, binaryPath, args...)
	defer cancel()
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		failTimedCommand(t, cmd, err, output)
	}

	return output
}

func runStopIfRunning(t *testing.T, binaryPath string, instance string) {
	t.Helper()

	cmd, cancel := newTimedCommand(t, binaryPath, "--output", "json", "scheduler", "stop", "--instance", instance)
	defer cancel()
	cmd.Env = os.Environ()
	_, _ = cmd.CombinedOutput()
}

func newTimedCommand(t *testing.T, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), jobsdCommandTimeout)
	return exec.CommandContext(ctx, name, args...), cancel
}

func failTimedCommand(t *testing.T, cmd *exec.Cmd, err error, output []byte) {
	t.Helper()

	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%s %v timed out after %s, output = %s", cmd.Path, cmd.Args[1:], jobsdCommandTimeout, output)
	}

	t.Fatalf("%s %v error = %v, output = %s", cmd.Path, cmd.Args[1:], err, output)
}

func waitForStateFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()

	waitForCondition(t, timeout, func() (bool, string) {
		_, err := daemon.ReadState(path)
		if err == nil {
			return true, ""
		}
		return false, err.Error()
	}, "state file %q was not created", path)
}

func waitForStateRemoval(t *testing.T, path string, timeout time.Duration) {
	t.Helper()

	waitForCondition(t, timeout, func() (bool, string) {
		_, err := daemon.ReadState(path)
		if errors.Is(err, daemon.ErrStateNotFound) {
			return true, ""
		}
		if err != nil {
			return false, err.Error()
		}
		return false, "state file still present"
	}, "state file %q was not removed", path)
}

func waitForRunStatus(
	t *testing.T,
	binaryPath string,
	instance string,
	runID int64,
	timeout time.Duration,
	wantedStatuses ...domain.RunStatus,
) runDetailOutput {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		detail := runJobsdJSON[runDetailOutput](t, binaryPath, "run", "get", "--instance", instance, "--run-id", fmt.Sprintf("%d", runID))
		status := domain.RunStatus(detail.Status)
		for _, wanted := range wantedStatuses {
			if status == wanted {
				return detail
			}
		}
		if status == domain.RunStatusSucceeded || status == domain.RunStatusFailed || status == domain.RunStatusCanceled {
			t.Fatalf("run %d reached terminal status %q, want one of %v", runID, detail.Status, wantedStatuses)
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("run %d did not reach status %v within %s", runID, wantedStatuses, timeout)
	return runDetailOutput{}
}

func waitForLatestRun(
	t *testing.T,
	binaryPath string,
	instance string,
	jobName string,
	timeout time.Duration,
	predicate func(runSummaryOutput) bool,
) runSummaryOutput {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		runs := runJobsdJSON[[]runSummaryOutput](t, binaryPath, "run", "list", "--instance", instance, "--job", jobName, "--limit", "20")
		for _, run := range runs {
			if predicate(run) {
				return run
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("run list for job %q did not satisfy predicate within %s", jobName, timeout)
	return runSummaryOutput{}
}

func waitForCondition(t *testing.T, timeout time.Duration, check func() (bool, string), format string, args ...any) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	lastReason := ""
	for time.Now().Before(deadline) {
		done, reason := check()
		if done {
			return
		}
		lastReason = reason
		time.Sleep(50 * time.Millisecond)
	}

	if lastReason != "" {
		t.Fatalf(format+" (last error: %s)", append(args, lastReason)...)
	}
	t.Fatalf(format, args...)
}

func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("Addr() type = %T, want *net.TCPAddr", listener.Addr())
	}

	return addr.Port
}

func bytesTrimSpace(data []byte) []byte {
	start := 0
	for start < len(data) && (data[start] == ' ' || data[start] == '\n' || data[start] == '\r' || data[start] == '\t') {
		start++
	}

	end := len(data)
	for end > start && (data[end-1] == ' ' || data[end-1] == '\n' || data[end-1] == '\r' || data[end-1] == '\t') {
		end--
	}

	return data[start:end]
}
