//go:build windows

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
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/daemon"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

const windowsCommandTimeout = 30 * time.Second

type schedulerCommandOutput struct {
	Instance string                 `json:"instance"`
	Status   domain.SchedulerStatus `json:"status"`
	Reason   string                 `json:"reason"`
}

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

	startOut := runJobsdJSON(t, binaryPath, "scheduler", "start", "--instance", firstInstance, "--port", fmt.Sprintf("%d", firstPort))
	if startOut.Status != domain.SchedulerStatusRunning {
		t.Fatalf("start status = %q, want %q", startOut.Status, domain.SchedulerStatusRunning)
	}

	firstPaths, err := config.ResolvePaths(firstInstance)
	if err != nil {
		t.Fatalf("ResolvePaths() first error = %v", err)
	}
	waitForStateFile(t, firstPaths.StatePath)

	statusOut := runJobsdJSON(t, binaryPath, "scheduler", "status", "--instance", firstInstance)
	if statusOut.Status != domain.SchedulerStatusRunning {
		t.Fatalf("status = %q, want %q", statusOut.Status, domain.SchedulerStatusRunning)
	}

	pingOut := runJobsdJSON(t, binaryPath, "scheduler", "ping", "--instance", firstInstance)
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
	waitForStateFile(t, secondPaths.StatePath)

	secondStatusOut := runJobsdJSON(t, binaryPath, "scheduler", "status", "--instance", secondInstance)
	if secondStatusOut.Status != domain.SchedulerStatusRunning {
		t.Fatalf("second status = %q, want %q", secondStatusOut.Status, domain.SchedulerStatusRunning)
	}

	stopOut := runJobsdJSON(t, binaryPath, "scheduler", "stop", "--instance", firstInstance)
	if stopOut.Status != domain.SchedulerStatusStopped {
		t.Fatalf("stop status = %q, want %q", stopOut.Status, domain.SchedulerStatusStopped)
	}
	waitForStateRemoval(t, firstPaths.StatePath)

	stoppedOut := runJobsdJSON(t, binaryPath, "scheduler", "status", "--instance", firstInstance)
	if stoppedOut.Status != domain.SchedulerStatusStopped {
		t.Fatalf("post-stop status = %q, want %q", stoppedOut.Status, domain.SchedulerStatusStopped)
	}
}

func buildJobsdBinary(t *testing.T) string {
	t.Helper()

	binaryPath := filepath.Join(t.TempDir(), "jobsd.exe")
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

func runJobsdJSON(t *testing.T, binaryPath string, args ...string) schedulerCommandOutput {
	t.Helper()

	output := runJobsd(t, binaryPath, append([]string{"--output", "json"}, args...)...)

	var payload schedulerCommandOutput
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v, output = %s", err, output)
	}

	return payload
}

func runJobsdExpectError(binaryPath string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), windowsCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command timed out after %s: %s %v", windowsCommandTimeout, binaryPath, args)
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

	ctx, cancel := context.WithTimeout(context.Background(), windowsCommandTimeout)
	return exec.CommandContext(ctx, name, args...), cancel
}

func failTimedCommand(t *testing.T, cmd *exec.Cmd, err error, output []byte) {
	t.Helper()

	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%s %v timed out after %s, output = %s", cmd.Path, cmd.Args[1:], windowsCommandTimeout, output)
	}

	t.Fatalf("%s %v error = %v, output = %s", cmd.Path, cmd.Args[1:], err, output)
}

func waitForStateFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := daemon.ReadState(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("state file %q was not created", path)
}

func waitForStateRemoval(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := daemon.ReadState(path); errors.Is(err, daemon.ErrStateNotFound) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("state file %q was not removed", path)
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
