package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/daemon"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestSchedulerCommandStructure(t *testing.T) {
	cmd := newSchedulerCommand(BuildInfo{})

	children := cmd.Commands()
	gotNames := make([]string, 0, len(children))
	hidden := map[string]bool{}
	for _, child := range children {
		gotNames = append(gotNames, child.Name())
		hidden[child.Name()] = child.Hidden
	}

	slices.Sort(gotNames)
	wantNames := []string{"ping", "serve", "start", "status", "stop"}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("command names = %#v, want %#v", gotNames, wantNames)
	}
	if !hidden["serve"] {
		t.Fatal("serve command Hidden = false, want true")
	}
}

func TestSchedulerCommandsRequireFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "start missing instance",
			args: []string{"scheduler", "start", "--port", "8080"},
			want: `required flag(s) "instance" not set`,
		},
		{
			name: "start missing port",
			args: []string{"scheduler", "start", "--instance", "dev"},
			want: `required flag(s) "port" not set`,
		},
		{
			name: "status missing instance",
			args: []string{"scheduler", "status"},
			want: `required flag(s) "instance" not set`,
		},
		{
			name: "stop missing instance",
			args: []string{"scheduler", "stop"},
			want: `required flag(s) "instance" not set`,
		},
		{
			name: "ping missing instance",
			args: []string{"scheduler", "ping"},
			want: `required flag(s) "instance" not set`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTestDirs(t)

			cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, &bytes.Buffer{}, &bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if err.Error() != tt.want {
				t.Fatalf("Execute() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestSchedulerStartReexecsServeModeAndWaitsForReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows lifecycle is covered by cmd/jobsd integration tests")
	}

	setTestDirs(t)

	originalStartServeProcess := startServeProcess
	originalStartupTimeout := schedulerStartupTimeout
	originalPollInterval := schedulerPollInterval
	t.Cleanup(func() {
		startServeProcess = originalStartServeProcess
		schedulerStartupTimeout = originalStartupTimeout
		schedulerPollInterval = originalPollInterval
	})

	schedulerStartupTimeout = 2 * time.Second
	schedulerPollInterval = 10 * time.Millisecond

	port := freePort(t)
	var gotExecutable string
	var gotArgs []string
	var cancel context.CancelFunc
	var errCh chan error

	startServeProcess = func(ctx context.Context, executable string, args []string) error {
		gotExecutable = executable
		gotArgs = append([]string(nil), args...)

		instance, port := parseServeArgs(t, args)
		paths, err := config.ResolvePaths(instance)
		if err != nil {
			t.Fatalf("ResolvePaths() error = %v", err)
		}

		serveCtx, serveCancel := context.WithCancel(context.Background())
		cancel = serveCancel
		errCh = make(chan error, 1)
		go func() {
			errCh <- daemon.Serve(serveCtx, daemon.ServeOptions{
				Instance: instance,
				Port:     port,
				Paths:    paths,
				Version:  "v1.0.0",
				Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
		}()

		return nil
	}

	stdout := &bytes.Buffer{}
	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, stdout, &bytes.Buffer{})
	cmd.SetArgs([]string{"scheduler", "start", "--instance", "dev", "--port", fmt.Sprintf("%d", port)})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotExecutable == "" {
		t.Fatal("startServeProcess executable = empty, want non-empty")
	}

	wantArgs := []string{"scheduler", "serve", "--instance", "dev", "--port", fmt.Sprintf("%d", port)}
	if !slices.Equal(gotArgs, wantArgs) {
		t.Fatalf("startServeProcess args = %#v, want %#v", gotArgs, wantArgs)
	}
	if !strings.Contains(stdout.String(), "running") {
		t.Fatalf("stdout = %q, want running status", stdout.String())
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestSchedulerStartSurfacesProcessError(t *testing.T) {
	setTestDirs(t)

	originalStartServeProcess := startServeProcess
	t.Cleanup(func() {
		startServeProcess = originalStartServeProcess
	})

	startServeProcess = func(ctx context.Context, executable string, args []string) error {
		return errors.New("boom")
	}

	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"scheduler", "start", "--instance", "dev", "--port", "8080"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if got, want := err.Error(), "start scheduler daemon: boom"; got != want {
		t.Fatalf("Execute() error = %q, want %q", got, want)
	}
}

func TestSchedulerStartTimesOutWhenDaemonNeverBecomesReady(t *testing.T) {
	setTestDirs(t)

	originalStartServeProcess := startServeProcess
	originalStartupTimeout := schedulerStartupTimeout
	originalPollInterval := schedulerPollInterval
	t.Cleanup(func() {
		startServeProcess = originalStartServeProcess
		schedulerStartupTimeout = originalStartupTimeout
		schedulerPollInterval = originalPollInterval
	})

	schedulerStartupTimeout = 50 * time.Millisecond
	schedulerPollInterval = 10 * time.Millisecond

	startServeProcess = func(ctx context.Context, executable string, args []string) error {
		return nil
	}

	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"scheduler", "start", "--instance", "dev", "--port", "8080"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "scheduler did not become ready") {
		t.Fatalf("Execute() error = %q, want startup timeout", err.Error())
	}
}

func TestSchedulerStatusClassifiesRunningStaleAndStopped(t *testing.T) {
	setTestDirs(t)

	t.Run("running", func(t *testing.T) {
		instance := "running"
		_, cancel, errCh := startTestDaemon(t, instance)
		defer stopTestDaemon(t, cancel, errCh)

		stdout := &bytes.Buffer{}
		cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, stdout, &bytes.Buffer{})
		cmd.SetArgs([]string{"--output", "json", "scheduler", "status", "--instance", instance})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		var payload schedulerOutput
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if payload.Status != domain.SchedulerStatusRunning {
			t.Fatalf("payload.Status = %q, want %q", payload.Status, domain.SchedulerStatusRunning)
		}
	})

	t.Run("stale", func(t *testing.T) {
		instance := "stale"
		paths, err := config.ResolvePaths(instance)
		if err != nil {
			t.Fatalf("ResolvePaths() error = %v", err)
		}

		if err := daemon.WriteState(paths.StatePath, domain.SchedulerState{
			Instance:  instance,
			PID:       4242,
			Port:      freePort(t),
			Token:     "stale-token",
			DBPath:    filepath.Join(paths.DataDir, "jobs.db"),
			StartedAt: time.Now().UTC(),
			Version:   "v1.0.0",
		}); err != nil {
			t.Fatalf("WriteState() error = %v", err)
		}

		stdout := &bytes.Buffer{}
		cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, stdout, &bytes.Buffer{})
		cmd.SetArgs([]string{"--output", "json", "scheduler", "status", "--instance", instance})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		var payload schedulerOutput
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if payload.Status != domain.SchedulerStatusStale {
			t.Fatalf("payload.Status = %q, want %q", payload.Status, domain.SchedulerStatusStale)
		}
	})

	t.Run("stopped", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, stdout, &bytes.Buffer{})
		cmd.SetArgs([]string{"--output", "json", "scheduler", "status", "--instance", "stopped"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		var payload schedulerOutput
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if payload.Status != domain.SchedulerStatusStopped {
			t.Fatalf("payload.Status = %q, want %q", payload.Status, domain.SchedulerStatusStopped)
		}
	})
}

func TestSchedulerPingSupportsTableAndJSON(t *testing.T) {
	setTestDirs(t)

	instance := "dev"
	_, cancel, errCh := startTestDaemon(t, instance)
	defer stopTestDaemon(t, cancel, errCh)

	t.Run("table", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, stdout, &bytes.Buffer{})
		cmd.SetArgs([]string{"scheduler", "ping", "--instance", instance})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if !strings.Contains(stdout.String(), "running") {
			t.Fatalf("stdout = %q, want running status", stdout.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, stdout, &bytes.Buffer{})
		cmd.SetArgs([]string{"--output", "json", "scheduler", "ping", "--instance", instance})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		var payload schedulerOutput
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if payload.Status != domain.SchedulerStatusRunning {
			t.Fatalf("payload.Status = %q, want %q", payload.Status, domain.SchedulerStatusRunning)
		}
	})
}

func TestSchedulerPingFailsWhenInstanceIsNotHealthy(t *testing.T) {
	setTestDirs(t)

	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, &bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"scheduler", "ping", "--instance", "missing"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if got, want := err.Error(), `instance "missing" is stopped`; got != want {
		t.Fatalf("Execute() error = %q, want %q", got, want)
	}
}

func TestSchedulerHelpIncludesJobsdExamples(t *testing.T) {
	setTestDirs(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "scheduler command",
			args: []string{"scheduler", "--help"},
			want: "jobsd scheduler start --instance dev --port 8080",
		},
		{
			name: "scheduler ping",
			args: []string{"scheduler", "ping", "--help"},
			want: "jobsd scheduler ping --instance dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, err := executeRootCommand(t, tt.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("stdout = %q, want substring %q", stdout, tt.want)
			}
		})
	}
}

func TestSchedulerTableOutputIsStable(t *testing.T) {
	setTestDirs(t)

	instance := "dev"
	_, cancel, errCh := startTestDaemon(t, instance)
	defer stopTestDaemon(t, cancel, errCh)

	stdout, err := executeRootCommand(t, "scheduler", "ping", "--instance", instance)
	if err != nil {
		t.Fatalf("scheduler ping error = %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("scheduler ping output lines = %d, want 2: %q", len(lines), stdout)
	}
	headerFields := strings.Fields(lines[0])
	wantHeaderFields := []string{"INSTANCE", "STATUS", "PID", "PORT", "DB_PATH", "STARTED_AT", "VERSION", "REASON"}
	if !slices.Equal(headerFields, wantHeaderFields) {
		t.Fatalf("header fields = %#v, want %#v", headerFields, wantHeaderFields)
	}
	rowFields := strings.Fields(lines[1])
	if len(rowFields) != 6 {
		t.Fatalf("row fields length = %d, want 6: %#v", len(rowFields), rowFields)
	}
	if rowFields[0] != "dev" || rowFields[1] != "running" || rowFields[5] != "v1.0.0" {
		t.Fatalf("data row fields = %#v", rowFields)
	}
}

func TestSchedulerStopShutsDownDaemon(t *testing.T) {
	setTestDirs(t)

	instance := "dev"
	paths, cancel, errCh := startTestDaemon(t, instance)

	stdout := &bytes.Buffer{}
	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, stdout, &bytes.Buffer{})
	cmd.SetArgs([]string{"--output", "json", "scheduler", "stop", "--instance", instance})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var payload schedulerOutput
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload.Status != domain.SchedulerStatusStopped {
		t.Fatalf("payload.Status = %q, want %q", payload.Status, domain.SchedulerStatusStopped)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	cancel()

	if _, err := daemon.ReadState(paths.StatePath); !errors.Is(err, daemon.ErrStateNotFound) {
		t.Fatalf("ReadState() error = %v, want %v", err, daemon.ErrStateNotFound)
	}
}

func TestSchedulerCommandShowsHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	cmd := NewRootCommand(BuildInfo{Version: "v1.0.0"}, stdout, &bytes.Buffer{})
	cmd.SetArgs([]string{"scheduler"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Manage scheduler lifecycle commands") {
		t.Fatalf("stdout = %q, want scheduler help output", stdout.String())
	}
}

func startTestDaemon(t *testing.T, instance string) (config.Paths, context.CancelFunc, <-chan error) {
	t.Helper()

	paths, err := config.ResolvePaths(instance)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Serve(ctx, daemon.ServeOptions{
			Instance: instance,
			Port:     port,
			Paths:    paths,
			Version:  "v1.0.0",
			Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
	}()

	waitForSchedulerState(t, paths.StatePath)
	waitForSchedulerRunning(t, instance)

	return paths, cancel, errCh
}

func stopTestDaemon(t *testing.T, cancel context.CancelFunc, errCh <-chan error) {
	t.Helper()

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func waitForSchedulerState(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := daemon.ReadState(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("state file %q was not created", path)
}

func waitForSchedulerRunning(t *testing.T, instance string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		inspection, err := inspectScheduler(context.Background(), instance)
		if err == nil && inspection.Status == domain.SchedulerStatusRunning {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("scheduler instance %q did not become running", instance)
}

func setTestDirs(t *testing.T) {
	t.Helper()

	baseDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(baseDir, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(baseDir, "runtime"))
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

func parseServeArgs(t *testing.T, args []string) (string, int) {
	t.Helper()

	var instance string
	var port int
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--instance":
			index++
			instance = args[index]
		case "--port":
			index++
			value := args[index]
			parsedPort, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("Atoi() error = %v", err)
			}
			port = parsedPort
		}
	}

	if instance == "" || port == 0 {
		t.Fatalf("args = %#v, want instance and port", args)
	}

	return instance, port
}
