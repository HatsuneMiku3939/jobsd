package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/sqlite"
)

func TestServeWritesStateAndMetadata(t *testing.T) {
	paths := testPaths(t, "dev")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- Serve(ctx, ServeOptions{
			Instance: "dev",
			Port:     8080,
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
	if state.Port != 8080 {
		t.Fatalf("state.Port = %d, want %d", state.Port, 8080)
	}
	if state.Token != "" {
		t.Fatalf("state.Token = %q, want empty string", state.Token)
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
	if meta.SchedulerPort != 8080 {
		t.Fatalf("meta.SchedulerPort = %d, want %d", meta.SchedulerPort, 8080)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	if _, err := ReadState(paths.StatePath); !errors.Is(err, ErrStateNotFound) {
		t.Fatalf("ReadState() after shutdown error = %v, want %v", err, ErrStateNotFound)
	}
}

func TestServeRejectsDuplicateInstance(t *testing.T) {
	paths := testPaths(t, "dev")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- Serve(ctx, ServeOptions{
			Instance: "dev",
			Port:     8080,
			Paths:    paths,
			Version:  "v1.0.0",
			Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
	}()

	waitForStateFile(t, paths.StatePath)

	err := Serve(context.Background(), ServeOptions{
		Instance: "dev",
		Port:     8081,
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

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestServeAllowsIndependentInstances(t *testing.T) {
	firstPaths := testPaths(t, "first")
	secondPaths := testPaths(t, "second")

	firstCtx, firstCancel := context.WithCancel(context.Background())
	secondCtx, secondCancel := context.WithCancel(context.Background())
	firstErrCh := make(chan error, 1)
	secondErrCh := make(chan error, 1)

	go func() {
		firstErrCh <- Serve(firstCtx, ServeOptions{
			Instance: "first",
			Port:     8080,
			Paths:    firstPaths,
			Version:  "v1.0.0",
		})
	}()
	go func() {
		secondErrCh <- Serve(secondCtx, ServeOptions{
			Instance: "second",
			Port:     8081,
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

	runServeOnce(t, paths, 8080)

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
	runServeOnce(t, paths, 9090)

	secondMeta, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if !secondMeta.CreatedAt.Equal(firstMeta.CreatedAt) {
		t.Fatalf("CreatedAt changed: first=%v second=%v", firstMeta.CreatedAt, secondMeta.CreatedAt)
	}
	if secondMeta.SchedulerPort != 9090 {
		t.Fatalf("SchedulerPort = %d, want %d", secondMeta.SchedulerPort, 9090)
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
		if err == nil {
			if state != (domain.SchedulerState{}) {
				return
			}
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
