# jobsd Full v1 Implementation Plan

## Summary

This document captures the implementation plan for the `jobsd` project
based on the current product documents in `docs/v1`:

- `CONCEPT.md`
- `CLI.md`
- `ARCHITECTURE.md`
- `SCHEMA.md`

The repository is currently greenfield, so this plan is intended to be
decision-complete and executable without additional design decisions.
The target scope is full v1, including `job update`, `scheduler ping`,
and `instance_metadata`, not just the minimum viable subset.

The binary name is fixed to `jobsd` to avoid conflicts with the Linux
shell built-in `jobs` command.

## Locked Decisions

- The project ships as a single binary named `jobsd`.
- `jobsd scheduler start` launches a detached background daemon by
  re-executing the same binary in an internal serve mode.
- Scheduler lifecycle operations use a local HTTP JSON control API bound to `127.0.0.1:<port>`.
- Job and run persistence use one SQLite database per instance.
- Supported schedule kinds for v1 are:
  - `every <duration>` for interval schedules
  - `cron <expr>` for five-field cron schedules
  - `after <duration>` for one-time schedules
- Logs use `log/slog`.
- SQLite uses `modernc.org/sqlite` to avoid a CGO requirement.
- The CLI supports `--output table|json`, defaulting to `table`.

## Public CLI Surface

The public command tree is:

```text
jobsd
├── scheduler
│   ├── start
│   ├── status
│   ├── stop
│   └── ping
├── job
│   ├── add
│   ├── list
│   ├── get
│   ├── update
│   ├── delete
│   ├── pause
│   ├── resume
│   └── run
├── run
│   ├── list
│   └── get
└── version
```

## Public Control API

The daemon exposes a loopback-only control API for scheduler lifecycle
and health operations.

### Bind Address

- `127.0.0.1:<port>`

### Authentication

- Each instance writes a random token to its runtime state file.
- CLI requests send the token in the `X-Jobs-Token` header.

### Endpoints

- `GET /v1/ping`
  - Returns a minimal health response.
- `GET /v1/scheduler`
  - Returns instance status, port, PID, database path, and start time.
- `POST /v1/scheduler/shutdown`
  - Requests graceful shutdown.

## Runtime and Storage Layout

Each instance uses isolated persistent and runtime paths.

- Database path: `~/.local/share/jobsd/instances/<instance>/jobs.db`
- Lock file: `${XDG_RUNTIME_DIR}/jobsd/<instance>.lock`
- Runtime directory: `${XDG_RUNTIME_DIR}/jobsd/<instance>/`
- State file: `${XDG_RUNTIME_DIR}/jobsd/<instance>/state.json`
- Runtime fallback without `XDG_RUNTIME_DIR`:
  `${TMPDIR:-/tmp}/jobsd-<uid>/<instance>/`

The instance name must match `[a-zA-Z0-9._-]+`.

The state file format is fixed to the following fields:

- `instance`
- `pid`
- `port`
- `token`
- `db_path`
- `started_at`
- `version`

## Repository Layout

The implementation follows this structure:

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

## Domain Types

The minimum shared domain types are:

- `Job`
- `Run`
- `Schedule`
- `ScheduleKind`
- `RunStatus`
- `ConcurrencyPolicy`
- `SchedulerState`

Locked enum values:

- `ScheduleKind`: `interval`, `cron`, `once`
- `RunStatus`: `pending`, `running`, `succeeded`, `failed`, `canceled`
- `ConcurrencyPolicy`: `forbid`, `queue`, `replace`

## Database Schema Scope

The initial migration must create:

- `schema_migrations`
- `jobs`
- `job_runs`
- `job_run_outputs`
- `instance_metadata`

### SQLite Configuration

Apply these pragmas when opening the database:

- `PRAGMA foreign_keys = ON`
- `PRAGMA journal_mode = WAL`
- `PRAGMA synchronous = NORMAL`
- `PRAGMA busy_timeout = 5000`

### Time Format

- Store all timestamps as UTC RFC3339 strings.
- Convert to local time only in CLI presentation.

## Implementation Phases

### 1. Bootstrap the Project

- Create `go.mod`.
- Add the binary entrypoint at `cmd/jobsd/main.go`.
- Add dependencies:
  - `github.com/spf13/cobra`
  - `modernc.org/sqlite`
  - `github.com/robfig/cron/v3`
- Keep `main.go` minimal and delegate command construction to `internal/app`.

### 2. Implement Path Resolution

- Add `internal/config/paths.go`.
- Resolve data, runtime, database, lock, and state paths from `--instance`.
- Create directories lazily when needed.
- Reject invalid instance names before any filesystem work.

### 3. Implement SQLite Setup and Migration

- Add `migrations/sqlite/0001_init.sql`.
- Add `internal/sqlite/db.go` for database open and pragma setup.
- Add `internal/sqlite/migrate.go` to apply numbered migrations deterministically.
- Initialize `instance_metadata` keys:
  - `instance_name`
  - `created_at`
  - `scheduler_port`

### 4. Implement Stores

#### Job Store

The job store must support:

- `Create`
- `GetByName`
- `List`
- `Update`
- `DeleteByName`
- `ListDue`
- `UpdateNextRun`
- `UpdateLastRunSummary`

#### Run Store

The run store must support:

- `EnqueueManual`
- `EnqueueScheduled`
- `ClaimPending`
- `MarkRunning`
- `MarkFinished`
- `CancelPendingByJob`
- `List`
- `Get`

#### Output Handling

- Persist execution output in `job_run_outputs`.
- Use upsert semantics on run completion.
- Store stdout and stderr separately.

### 5. Implement Schedule Parsing

Support these user-facing forms:

- `every 10m`
- `every 1h30m`
- `cron */5 * * * *`
- `after 10m`

Rules:

- Interval schedules compute the next time from the current reference time.
- Cron schedules use five fields only.
- One-time schedules are normalized to `ScheduleKind = once`.
- A one-time job is automatically disabled after its first scheduled
  enqueue by setting:
  - `enabled = 0`
  - `next_run_at = NULL`

### 6. Implement the Lock Layer

- Add `internal/lock/filelock.go`.
- Use an OS-level file lock per instance.
- Starting the same instance twice must fail before the daemon becomes available.
- Different instances may run concurrently.

### 7. Implement Daemon Start and Serve Flow

The daemon flow is fixed as follows:

1. `jobsd scheduler start` validates input and resolves paths.
2. The parent process re-executes the same binary in a hidden internal
   mode such as `jobsd scheduler serve`.
3. The child process:
   - acquires the lock
   - opens or creates the database
   - runs migrations
   - writes `instance_metadata`
   - writes the runtime state file
   - starts the HTTP control server
   - starts the scheduler loop
4. The parent polls `GET /v1/ping` until healthy or timeout.
5. The parent prints success and exits.

Phase 9 may stage this flow in smaller internal steps.
In that stage, the daemon may first implement lock acquisition,
database initialization, metadata persistence, and state file lifecycle
before the HTTP control server and scheduler loop are attached.

On shutdown, the daemon must:

- stop accepting new work
- request loop shutdown
- release the lock
- remove the runtime state file

### 8. Implement Scheduler Control Commands

#### `scheduler status`

- Read the runtime state file if it exists.
- Call `GET /v1/scheduler` if the state file contains a reachable port and token.
- Classify the result as:
  - `running`
  - `stale`
  - `stopped`

#### `scheduler stop`

- Call `POST /v1/scheduler/shutdown`.
- Wait until the process exits or a timeout is reached.
- Confirm the state file is removed.

#### `scheduler ping`

- Return a machine-friendly response.
- Exit with code `0` when reachable and healthy.
- Exit with a non-zero code otherwise.
- Support both table and JSON output formats.

### 9. Implement the Scheduler Loop

The loop runs every second and performs the following steps:

1. Query due jobs where:
   - `enabled = 1`
   - `next_run_at <= now`
2. Apply concurrency policy per job.
3. Enqueue scheduled runs.
4. Recompute and persist `next_run_at`.
5. Claim pending runs for execution.
6. Mark claimed runs as running.
7. Execute jobs.
8. Persist completion state and captured output.
9. Update denormalized job summary fields.

### 10. Implement the Executor

- Use shell execution:
  - Unix: `sh -lc`
  - Windows: `cmd /C`
- Capture stdout and stderr independently.
- Limit each output stream to `64 KiB`.
- Set truncation flags when the limit is exceeded.
- Return a terminal execution result object so the scheduler loop can map
  it directly into run completion persistence.
- Persist:
  - final status
  - exit code
  - error message
  - started and finished timestamps

### 11. Lock Concurrency Policy Behavior

The policy behavior is fixed and must not be reinterpreted during implementation.

#### `forbid`

- If the job already has a `pending` or `running` run:
  - manual trigger returns a conflict error
  - scheduled trigger is skipped
- Scheduled skip still recomputes the next schedule.

#### `queue`

- Always enqueue a new `pending` run.

#### `replace`

- Cancel existing `pending` runs for the same job.
- If a matching run is currently executing:
  - cancel the running process through an in-memory cancel handle
  - mark the replaced run as `canceled`
- Enqueue the new run afterward.

The daemon must therefore keep in-memory execution handles keyed by run
ID or job ID.

### 12. Implement Job Commands

#### `job add`

- Validate all required flags.
- Parse schedule before writing anything.
- Compute initial `next_run_at` unless the job is disabled.

#### `job list`

- Show:
  - name
  - enabled state
  - schedule
  - next run time
  - last run time

#### `job get`

- Return the full persisted definition and denormalized status fields.

#### `job update`

- Update only the fields explicitly provided.
- Re-parse and re-normalize schedule fields when changed.
- Recompute `next_run_at` when schedule, timezone, or enabled state changes.

#### `job delete`

- Delete by job name.
- Cascade delete run and output records through foreign keys.

#### `job pause`

- Set `enabled = 0`.
- Set `updated_at`.
- Preserve historical runs.

#### `job resume`

- Set `enabled = 1`.
- Recompute `next_run_at` from the current time.

#### `job run`

- Insert a manual `pending` run.
- Return the new run ID immediately.
- Do not bypass the normal scheduler execution path.
- Allow enqueue even if the daemon is not running.

### 13. Implement Run Commands

#### `run list`

Support filters:

- `--job`
- `--status`
- `--limit`

List output includes:

- run ID
- job name
- trigger type
- status
- started time
- finished time
- duration

#### `run get`

Return:

- run metadata
- exit code
- error message
- output truncation metadata
- stdout summary
- stderr summary

### 14. Output Formatting

- Keep `internal/output` small.
- Support:
  - human-readable table output
  - JSON output for scripts
- Reuse the same DTOs across command handlers where possible.

### 15. Version Reporting

- Add build-time variables for:
  - version
  - commit
  - build date
- `jobsd version` prints those values in table or JSON form.

### 16. Documentation

Add or update:

- `README.md`
- `CONCEPT.md`
- `CLI.md`

Documentation must:

- use `jobsd` consistently
- describe the instance model
- document schedule grammar
- explain runtime and data directory layout
- show the common workflow
- mention the local control API behavior

## Test Plan

All code changes must include tests. Favor table-driven tests where practical.

### Unit Tests

- Schedule parser:
  - valid interval syntax
  - valid cron syntax
  - valid one-time syntax
  - invalid expressions
  - next-run calculation
- Config paths:
  - XDG resolution
  - default fallback behavior
  - invalid instance names
- Lock behavior:
  - duplicate instance rejection
  - different instance success
- Output truncation logic
- Concurrency policy decision logic

### Integration Tests

- Migration applies successfully to an empty database.
- Job CRUD works against a temporary SQLite database.
- Unique job names are enforced.
- Run enqueue and retrieval work correctly.
- Run output rows are written on completion.
- Cascade delete removes job runs and outputs.
- `instance_metadata` persists expected values.

### Daemon Tests

- Due jobs are enqueued on schedule.
- Manual runs are claimed and executed.
- `next_run_at` is updated after scheduled enqueue.
- One-time jobs disable themselves after first scheduled enqueue.
- `replace` cancels in-flight executions.
- Graceful shutdown releases lock and removes state file.

### CLI Tests

- Required flag validation
- JSON output shape
- Human-readable output smoke tests
- Correct exit codes for success and failure
- Binary/help text uses `jobsd`

### End-to-End Tests

- `scheduler start -> job add -> job run -> run list -> run get -> scheduler stop`
- automatic interval execution
- automatic cron execution
- one-time schedule executes once only
- duplicate `scheduler start` for the same instance fails

## Acceptance Criteria

- One instance never reads or writes another instance database.
- Starting the same instance twice fails because of the lock layer.
- The database is created automatically when starting a new instance.
- Manual and scheduled runs share the same execution and tracking path.
- Scheduler lifecycle commands work without a global instance registry.
- Run history and captured output persist in SQLite.
- The CLI and documentation consistently use `jobsd`.
- GitHub Actions validates the project on Linux, macOS, and Windows.
- Windows lifecycle behavior is verified on a real Windows runner by
  starting, inspecting, pinging, and stopping a detached daemon.

## Assumptions and Defaults

- The implementation starts from an empty repository with only planning documents.
- Unix is the primary target for daemon backgrounding behavior.
- Windows support keeps the same product shape but may differ internally
  for shell execution and daemon lifecycle internals.
- Windows runtime verification relies on GitHub Actions because detached
  process behavior and file locking cannot be validated from a Linux-only
  development environment.
- Cron syntax is limited to five fields and excludes seconds.
- One-time scheduling supports only `after <duration>` in v1.
- Job CRUD is available even when the daemon is stopped.
- Actual execution occurs only when the daemon is running.

## Recommended Execution Order

Implement in this order:

1. bootstrap and module setup
2. config and path resolution
3. SQLite open and migration support
4. domain types and store layer
5. schedule parsing and next-run calculation
6. file locking
7. daemon serve mode and control API
8. scheduler lifecycle commands
8.5 Windows runtime support
9. scheduler loop and executor
10. CLI command handlers
11. tests
12. documentation polish

This order minimizes rework and keeps early milestones testable.
Windows runtime support belongs after scheduler lifecycle stabilization so
that process spawning, lock semantics, and shutdown behavior can align
with the final serve/status/stop flow before executor-specific Windows
shell behavior is added.
The validation for that phase should include a GitHub Actions matrix on
Linux, macOS, and Windows, with Windows-specific lifecycle tests gated by
`//go:build windows`.
