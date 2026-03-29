package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/sqlite"
)

func TestServeWritesStateAndMetadata(t *testing.T) {
	paths := testPaths(t, "dev")
	port := freePort(t)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- Serve(ctx, ServeOptions{
			Instance: "dev",
			Port:     port,
			Paths:    paths,
			Version:  "v1.0.0",
			Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
	}()

	waitForStateFile(t, paths.StatePath)

	state, err := ReadState(paths.StatePath)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	if state.Instance != "dev" {
		t.Fatalf("state.Instance = %q, want %q", state.Instance, "dev")
	}
	if state.Port != port {
		t.Fatalf("state.Port = %d, want %d", state.Port, port)
	}
	if state.Token == "" {
		t.Fatal("state.Token = empty, want non-empty")
	}

	db, err := sqlite.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	meta, err := sqlite.NewMetadataStore(db).Get(context.Background())
	if err != nil {
		t.Fatalf("Get() metadata error = %v", err)
	}
	if meta.InstanceName != "dev" {
		t.Fatalf("meta.InstanceName = %q, want %q", meta.InstanceName, "dev")
	}
	if meta.SchedulerPort != port {
		t.Fatalf("meta.SchedulerPort = %d, want %d", meta.SchedulerPort, port)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	if _, err := ReadState(paths.StatePath); !errors.Is(err, ErrStateNotFound) {
		t.Fatalf("ReadState() after shutdown error = %v, want %v", err, ErrStateNotFound)
	}
}

func TestServeControlAPISuccessAndAuthFailures(t *testing.T) {
	paths := testPaths(t, "dev")
	port := freePort(t)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ServeOptions{
			Instance: "dev",
			Port:     port,
			Paths:    paths,
			Version:  "v1.0.0",
			Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
	}()

	waitForStateFile(t, paths.StatePath)
	state, err := ReadState(paths.StatePath)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	waitForControlReady(t, port, state.Token)

	t.Run("ping success", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, controlURL(port, "/v1/ping"), nil)
		if err != nil {
			t.Fatalf("NewRequestWithContext() error = %v", err)
		}
		req.Header.Set(jobsTokenHeader, state.Token)

		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer response.Body.Close()

		if response.StatusCode != http.StatusOK {
			t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusOK)
		}

		var payload PingResponse
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload.Status != domain.SchedulerStatusRunning {
			t.Fatalf("payload.Status = %q, want %q", payload.Status, domain.SchedulerStatusRunning)
		}
	})

	t.Run("scheduler success", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, controlURL(port, "/v1/scheduler"), nil)
		if err != nil {
			t.Fatalf("NewRequestWithContext() error = %v", err)
		}
		req.Header.Set(jobsTokenHeader, state.Token)

		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer response.Body.Close()

		if response.StatusCode != http.StatusOK {
			t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusOK)
		}

		var payload SchedulerResponse
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload.DBPath != paths.DatabasePath {
			t.Fatalf("payload.DBPath = %q, want %q", payload.DBPath, paths.DatabasePath)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, controlURL(port, "/v1/ping"), nil)
		if err != nil {
			t.Fatalf("NewRequestWithContext() error = %v", err)
		}

		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer response.Body.Close()

		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, controlURL(port, "/v1/ping"), nil)
		if err != nil {
			t.Fatalf("NewRequestWithContext() error = %v", err)
		}
		req.Header.Set(jobsTokenHeader, "wrong-token")

		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer response.Body.Close()

		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusUnauthorized)
		}
	})

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestServeShutdownEndpointRemovesState(t *testing.T) {
	paths := testPaths(t, "dev")
	port := freePort(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ServeOptions{
			Instance: "dev",
			Port:     port,
			Paths:    paths,
			Version:  "v1.0.0",
			Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
	}()

	waitForStateFile(t, paths.StatePath)
	state, err := ReadState(paths.StatePath)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	waitForControlReady(t, port, state.Token)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, controlURL(port, "/v1/scheduler/shutdown"), nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	req.Header.Set(jobsTokenHeader, state.Token)

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusNoContent)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if _, err := ReadState(paths.StatePath); !errors.Is(err, ErrStateNotFound) {
		t.Fatalf("ReadState() after shutdown error = %v, want %v", err, ErrStateNotFound)
	}
}

func TestServeRejectsDuplicateInstance(t *testing.T) {
	paths := testPaths(t, "dev")
	firstPort := freePort(t)
	secondPort := freePort(t)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- Serve(ctx, ServeOptions{
			Instance: "dev",
			Port:     firstPort,
			Paths:    paths,
			Version:  "v1.0.0",
			Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
	}()

	waitForStateFile(t, paths.StatePath)
	state, err := ReadState(paths.StatePath)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	waitForControlReady(t, firstPort, state.Token)

	err = Serve(context.Background(), ServeOptions{
		Instance: "dev",
		Port:     secondPort,
		Paths:    paths,
		Version:  "v1.0.0",
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err == nil {
		t.Fatal("Serve() duplicate error = nil, want error")
	}
	if got, want := err.Error(), `instance "dev" is already running`; got != want {
		t.Fatalf("Serve() duplicate error = %q, want %q", got, want)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, controlURL(firstPort, "/v1/scheduler/shutdown"), nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	req.Header.Set(jobsTokenHeader, state.Token)

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusNoContent)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestServeAllowsIndependentInstances(t *testing.T) {
	firstPaths := testPaths(t, "first")
	secondPaths := testPaths(t, "second")
	firstPort := freePort(t)
	secondPort := freePort(t)

	firstCtx, firstCancel := context.WithCancel(context.Background())
	secondCtx, secondCancel := context.WithCancel(context.Background())
	firstErrCh := make(chan error, 1)
	secondErrCh := make(chan error, 1)

	go func() {
		firstErrCh <- Serve(firstCtx, ServeOptions{
			Instance: "first",
			Port:     firstPort,
			Paths:    firstPaths,
			Version:  "v1.0.0",
		})
	}()
	go func() {
		secondErrCh <- Serve(secondCtx, ServeOptions{
			Instance: "second",
			Port:     secondPort,
			Paths:    secondPaths,
			Version:  "v1.0.0",
		})
	}()

	waitForStateFile(t, firstPaths.StatePath)
	waitForStateFile(t, secondPaths.StatePath)

	firstCancel()
	secondCancel()

	if err := <-firstErrCh; err != nil {
		t.Fatalf("Serve() first error = %v", err)
	}
	if err := <-secondErrCh; err != nil {
		t.Fatalf("Serve() second error = %v", err)
	}
}

func TestServePreservesCreatedAtAndUpdatesPort(t *testing.T) {
	paths := testPaths(t, "dev")

	firstPort := freePort(t)
	runServeOnce(t, paths, firstPort)

	db, err := sqlite.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	store := sqlite.NewMetadataStore(db)
	firstMeta, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() first error = %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	secondPort := freePort(t)
	runServeOnce(t, paths, secondPort)

	secondMeta, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if !secondMeta.CreatedAt.Equal(firstMeta.CreatedAt) {
		t.Fatalf("CreatedAt changed: first=%v second=%v", firstMeta.CreatedAt, secondMeta.CreatedAt)
	}
	if secondMeta.SchedulerPort != secondPort {
		t.Fatalf("SchedulerPort = %d, want %d", secondMeta.SchedulerPort, secondPort)
	}
}

func runServeOnce(t *testing.T, paths config.Paths, port int) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- Serve(ctx, ServeOptions{
			Instance: paths.Instance,
			Port:     port,
			Paths:    paths,
			Version:  "v1.0.0",
		})
	}()

	waitForStateFile(t, paths.StatePath)
	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func waitForStateFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := ReadState(path)
		if err == nil && state != (domain.SchedulerState{}) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("state file %q was not created", path)
}

func testPaths(t *testing.T, instance string) config.Paths {
	t.Helper()

	baseDir := filepath.Join(t.TempDir(), instance)
	return config.Paths{
		Instance:     instance,
		DataDir:      filepath.Join(baseDir, "data"),
		RuntimeDir:   filepath.Join(baseDir, "runtime"),
		DatabasePath: filepath.Join(baseDir, "data", "jobs.db"),
		LockPath:     filepath.Join(baseDir, "runtime", instance+".lock"),
		StatePath:    filepath.Join(baseDir, "runtime", "state.json"),
	}
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

func controlURL(port int, path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
}

func waitForControlReady(t *testing.T, port int, token string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, controlURL(port, "/v1/ping"), nil)
		if err != nil {
			t.Fatalf("NewRequestWithContext() error = %v", err)
		}
		req.Header.Set(jobsTokenHeader, token)

		response, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("control api on port %d did not become ready", port)
}
