package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/lock"
	"github.com/hatsunemiku3939/jobsd/internal/sqlite"
)

type ServeOptions struct {
	Instance string
	Port     int
	Paths    config.Paths
	Version  string
	Logger   *slog.Logger
}

func Serve(ctx context.Context, opts ServeOptions) error {
	if opts.Instance == "" {
		return fmt.Errorf("instance is required")
	}
	if opts.Port <= 0 || opts.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if opts.Version == "" {
		return fmt.Errorf("version is required")
	}

	resolvedPaths := opts.Paths
	if resolvedPaths.Instance == "" {
		paths, err := config.ResolvePaths(opts.Instance)
		if err != nil {
			return fmt.Errorf("resolve paths: %w", err)
		}
		resolvedPaths = paths
	}
	if resolvedPaths.Instance != opts.Instance {
		return fmt.Errorf("paths instance %q does not match %q", resolvedPaths.Instance, opts.Instance)
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	fileLock, err := lock.Acquire(resolvedPaths.LockPath)
	if err != nil {
		if errors.Is(err, lock.ErrAlreadyLocked) {
			return fmt.Errorf("instance %q is already running", opts.Instance)
		}
		return fmt.Errorf("acquire instance lock: %w", err)
	}
	lockHeld := true
	defer func() {
		if lockHeld {
			_ = fileLock.Release()
		}
	}()

	db, err := sqlite.Open(resolvedPaths.DatabasePath)
	if err != nil {
		return fmt.Errorf("open instance database: %w", err)
	}
	dbOpen := true
	defer func() {
		if dbOpen {
			_ = db.Close()
		}
	}()

	if err := sqlite.Migrate(ctx, db); err != nil {
		return fmt.Errorf("migrate instance database: %w", err)
	}

	if err := upsertInstanceMetadata(ctx, sqlite.NewMetadataStore(db), opts.Instance, opts.Port); err != nil {
		return err
	}

	token, err := generateToken()
	if err != nil {
		return err
	}

	startedAt := time.Now().UTC()
	state := domain.SchedulerState{
		Instance:  opts.Instance,
		PID:       os.Getpid(),
		Port:      opts.Port,
		Token:     token,
		DBPath:    resolvedPaths.DatabasePath,
		StartedAt: startedAt,
		Version:   opts.Version,
	}
	if err := WriteState(resolvedPaths.StatePath, state); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	stateWritten := true
	defer func() {
		if stateWritten {
			_ = RemoveState(resolvedPaths.StatePath)
		}
	}()

	shutdownRequested := make(chan struct{}, 1)
	controlServer, controlErrCh, err := startControlServer(state, func() {
		select {
		case shutdownRequested <- struct{}{}:
		default:
		}
	})
	if err != nil {
		return err
	}

	loopCtx, cancelLoop := context.WithCancel(ctx)
	defer cancelLoop()

	loopErrCh := make(chan error, 1)
	go func() {
		loopErrCh <- (&Loop{
			JobStore: sqlite.NewJobStore(db),
			RunStore: sqlite.NewRunStore(db),
			Executor: ShellExecutor{},
			Logger:   logger,
		}).Run(loopCtx)
	}()

	signalCtx, stop := signal.NotifyContext(ctx, shutdownSignals()...)
	defer stop()

	logger.Info("scheduler serve started",
		slog.String("instance", opts.Instance),
		slog.Int("port", opts.Port),
		slog.String("db_path", resolvedPaths.DatabasePath),
	)

	shuttingDown := false
	select {
	case <-signalCtx.Done():
		shuttingDown = true
	case <-shutdownRequested:
		shuttingDown = true
	case err := <-controlErrCh:
		if err != nil {
			return err
		}
		shuttingDown = signalCtx.Err() != nil || ctx.Err() != nil
		if !shuttingDown {
			return fmt.Errorf("control api stopped unexpectedly")
		}
	case err := <-loopErrCh:
		if err != nil {
			return err
		}
		shuttingDown = signalCtx.Err() != nil || ctx.Err() != nil
		if !shuttingDown {
			return fmt.Errorf("scheduler loop stopped unexpectedly")
		}
	}

	if !shuttingDown {
		return fmt.Errorf("scheduler shutdown state was not reached")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdownControlServer(shutdownCtx, controlServer); err != nil {
		return err
	}
	cancelLoop()
	if err := <-controlErrCh; err != nil {
		return err
	}
	if err := <-loopErrCh; err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	if err := RemoveState(resolvedPaths.StatePath); err != nil {
		return fmt.Errorf("remove state file: %w", err)
	}
	stateWritten = false
	if err := fileLock.Release(); err != nil {
		return fmt.Errorf("release instance lock: %w", err)
	}
	lockHeld = false
	if err := db.Close(); err != nil {
		return fmt.Errorf("close instance database: %w", err)
	}
	dbOpen = false

	return nil
}

func upsertInstanceMetadata(ctx context.Context, store *sqlite.MetadataStore, instance string, port int) error {
	meta, err := store.Get(ctx)
	switch {
	case err == nil:
		if meta.InstanceName != instance {
			return fmt.Errorf("instance metadata mismatch: got %q want %q", meta.InstanceName, instance)
		}

		meta.SchedulerPort = port
		if err := store.Upsert(ctx, meta); err != nil {
			return fmt.Errorf("update instance metadata: %w", err)
		}
		return nil
	case errors.Is(err, sqlite.ErrMetadataNotFound):
		meta = domain.InstanceMetadata{
			InstanceName:  instance,
			CreatedAt:     time.Now().UTC(),
			SchedulerPort: port,
		}
		if err := store.Upsert(ctx, meta); err != nil {
			return fmt.Errorf("initialize instance metadata: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("read instance metadata: %w", err)
	}
}
