# jobsd Go Package Structure

## Goals

The package structure should:

- keep the CLI entrypoint small
- separate scheduler runtime logic from command wiring
- isolate SQLite access behind repository interfaces or focused store packages
- make unit testing easy
- stay simple for a first release

## Recommended Layout

```text
jobsd/
├── cmd/
│   └── jobsd/
│       └── main.go
├── internal/
│   ├── app/
│   │   ├── root.go
│   │   ├── scheduler.go
│   │   ├── job.go
│   │   ├── run.go
│   │   └── version.go
│   ├── config/
│   │   └── paths.go
│   ├── daemon/
│   │   ├── server.go
│   │   ├── loop.go
│   │   └── executor.go
│   ├── lock/
│   │   └── filelock.go
│   ├── schedule/
│   │   ├── parser.go
│   │   └── next.go
│   ├── sqlite/
│   │   ├── db.go
│   │   ├── migrate.go
│   │   ├── jobs_store.go
│   │   ├── runs_store.go
│   │   └── metadata_store.go
│   ├── domain/
│   │   ├── job.go
│   │   ├── run.go
│   │   └── types.go
│   └── output/
│       └── printer.go
└── migrations/
    └── sqlite/
        └── 0001_init.sql
```

## Package Responsibilities

### `cmd/jobsd`

Contains only the binary entrypoint.

Responsibilities:

- initialize the root command
- run the application
- convert top-level errors into exit codes

Keep `main.go` very small.

### `internal/app`

Contains CLI command construction and command handlers.

Responsibilities:

- define command tree and flags
- validate command input
- call lower-level services
- format command output

Suggested split:

- `root.go`: root command and shared flags
- `scheduler.go`: `scheduler start`, `status`, `stop`, `ping`
- `job.go`: `job add`, `list`, `get`, `update`, `delete`, `pause`, `resume`, `run`
- `run.go`: `run list`, `run get`
- `version.go`: version command

This package should not contain raw SQL.

### `internal/config`

Resolve instance-specific paths and runtime directories.

Responsibilities:

- build data directory paths from `--instance`
- build runtime directory paths from `--instance`
- resolve database path
- resolve lock file path

This package centralizes XDG path behavior so it is easy to test.

### `internal/daemon`

Contains the scheduler runtime.

Responsibilities:

- start and stop the scheduler process
- poll due jobs
- claim pending runs
- execute jobs
- update run state

Suggested split:

- `server.go`: lifecycle orchestration
- `loop.go`: ticker-driven scheduling loop
- `executor.go`: command execution, output capture, and terminal run results

This package should focus on orchestration, not SQL details.
Platform-specific daemon launch behavior may live behind small helpers so
Windows backgrounding can differ internally without changing the public
CLI contract.
Signal registration should also remain platform-specific so Windows does
not depend on Unix-only signals such as `SIGTERM`.

### `internal/lock`

Contains instance exclusivity logic.

Responsibilities:

- acquire instance lock file
- release lock on shutdown
- expose a small API for lock ownership

Keeping this separate makes it easier to test duplicate startup behavior.
Platform-specific lock implementations are acceptable so long as they
preserve the same duplicate-start rejection semantics for the caller.
On Windows, the lock implementation may use an exclusive file handle
instead of Unix-style `flock`, so long as duplicate startup is rejected
until the owning process releases the handle.

### `internal/schedule`

Contains schedule parsing and next-run calculation.

Responsibilities:

- parse user-friendly schedule expressions
- normalize schedule data for persistence
- calculate the next run time from a reference point

This package should stay independent from SQLite and CLI code.

### `internal/sqlite`

Contains SQLite connection setup, migrations, and store implementations.

Responsibilities:

- open SQLite connections with the right pragmas
- apply migrations
- implement job and run persistence
- expose narrow storage methods used by the daemon and CLI

Suggested store split:

- `db.go`: open database, pragma setup, shared helpers
- `migrate.go`: migration runner
- `jobs_store.go`: CRUD for jobs
- `runs_store.go`: queueing, claiming, and reading runs
- `metadata_store.go`: optional instance metadata access

### `internal/domain`

Contains core domain types shared across packages.

Suggested types:

- `Job`
- `Run`
- `Schedule`
- `RunStatus`
- `ConcurrencyPolicy`

This package should not depend on CLI or SQLite details.

### `internal/output`

Contains output formatting helpers.

Responsibilities:

- human-readable table output
- JSON output for scripts
- shared rendering helpers

Keeping presentation out of command handlers reduces duplication.

### `migrations/sqlite`

Contains SQL migration files.

Responsibilities:

- keep schema changes versioned
- allow deterministic database initialization

## Service Boundaries

The implementation can stay simple if it follows these boundaries:

```text
CLI command
  -> app handler
  -> domain/service operation
  -> sqlite store
  -> SQLite
```

And for runtime execution:

```text
daemon loop
  -> schedule calculation
  -> sqlite store
  -> executor
  -> sqlite store
```

## Suggested Internal Interfaces

These interfaces do not need to be introduced on day one, but they are useful seams.

```go
type JobStore interface {
    Create(ctx context.Context, job domain.Job) (domain.Job, error)
    GetByID(ctx context.Context, id int64) (domain.Job, error)
    GetByName(ctx context.Context, name string) (domain.Job, error)
    List(ctx context.Context) ([]domain.Job, error)
    Update(ctx context.Context, job domain.Job) (domain.Job, error)
    DeleteByName(ctx context.Context, name string) error
    ListDue(ctx context.Context, now time.Time) ([]domain.Job, error)
    UpdateNextRun(
        ctx context.Context,
        jobID int64,
        next *time.Time,
        updatedAt time.Time,
    ) error
    UpdateLastRunSummary(
        ctx context.Context,
        jobID int64,
        lastRunAt *time.Time,
        lastRunStatus *domain.RunStatus,
        updatedAt time.Time,
    ) error
}

type RunStore interface {
    EnqueueManual(
        ctx context.Context,
        jobID int64,
        queuedAt time.Time,
    ) (domain.Run, error)
    EnqueueScheduled(
        ctx context.Context,
        jobID int64,
        scheduledFor time.Time,
        queuedAt time.Time,
    ) (domain.Run, error)
    ListPending(
        ctx context.Context,
        limit int,
    ) ([]domain.Run, error)
    TryClaimPending(
        ctx context.Context,
        runID int64,
        runnerID string,
    ) (bool, error)
    MarkRunning(ctx context.Context, runID int64, startedAt time.Time) error
    CancelPendingByJob(ctx context.Context, jobID int64, canceledAt time.Time) error
    ListUnfinishedByJob(ctx context.Context, jobID int64) ([]domain.Run, error)
    MarkFinished(ctx context.Context, result FinishRunParams) error
    List(ctx context.Context, filter ListRunsFilter) ([]domain.Run, error)
    Get(ctx context.Context, runID int64) (domain.Run, error)
}
```

These interfaces support:

- CLI-driven job management
- scheduler-driven due job processing
- unified handling of scheduled and manual runs

The scheduler loop keeps in-memory execution handles keyed by run ID so it
can:

- cancel active runs during `replace`
- cancel all active runs during shutdown
- serialize active execution per job for the `queue` policy

## Testing Strategy

The package layout should make these tests natural:

- table tests for schedule parsing in `internal/schedule`
- repository tests against temporary SQLite databases in `internal/sqlite`
- daemon loop tests with fake stores and fake executors
- CLI handler tests for flag validation and command output

## First Version Recommendation

For the first implementation, start with:

- `cmd/jobsd`
- `internal/app`
- `internal/config`
- `internal/daemon`
- `internal/lock`
- `internal/schedule`
- `internal/sqlite`
- `internal/domain`

`internal/output` can stay very small at first, and richer formatting can
be added later.

## Practical Notes

### One binary, two roles

Use one `jobsd` binary for both:

- daemon lifecycle commands
- management commands

This keeps distribution simple and matches the CLI-first product concept.

### Keep services thin at first

It is fine to let `internal/app` call `internal/sqlite` directly in the
first version.
If business logic grows, extract service packages later.

### Avoid premature package splitting

Do not create separate packages for every noun.
Start with cohesive packages based on responsibility, then split only
when the code proves it is necessary.
