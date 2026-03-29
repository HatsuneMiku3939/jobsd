# jobsd v1 TODO

## How to Use This TODO

- Follow the tasks in order unless a later task explicitly says it can
  be done in parallel.
- Keep each checkbox scoped to one reviewable change.
- Add tests in the same change whenever a task introduces behavior.
- Update documentation when a task changes user-visible behavior or command examples.

## Phase 0: Repository Bootstrap

- [x] Create `go.mod` with the project module path.
- [x] Add the initial dependencies: `cobra`, `modernc.org/sqlite`, and `robfig/cron/v3`.
- [x] Create `cmd/jobsd/main.go` with a minimal entrypoint.
- [x] Add build-time version variables and wire them into the main package.
- [x] Add a `.gitignore` suitable for Go binaries, test artifacts, and editor files.

## Phase 1: Core Project Skeleton

- [x] Create the package directories under `internal/`.
- [x] Add empty root command wiring in `internal/app/root.go`.
- [x] Add placeholder command files for `scheduler`, `job`, `run`, and `version`.
- [x] Add `internal/domain/job.go` with the `Job` type.
- [x] Add `internal/domain/run.go` with the `Run` type.
- [x] Add `internal/domain/types.go` with shared enums and helper types.
- [x] Add `internal/output/printer.go` with table and JSON output scaffolding.

## Phase 2: Path and Runtime Layout

- [x] Implement instance name validation in `internal/config/paths.go`.
- [x] Implement data directory resolution for instance databases.
- [x] Implement runtime directory resolution for lock and state files.
- [x] Implement helper functions for database path, lock path, and state file path.
- [x] Add table-driven tests for valid and invalid instance names.
- [x] Add tests for XDG path resolution and fallback behavior.

## Phase 3: SQLite Foundation

- [x] Add `migrations/sqlite/0001_init.sql`.
- [x] Create `internal/sqlite/db.go` to open SQLite with the required pragmas.
- [x] Create `internal/sqlite/migrate.go` to apply the initial migration.
- [x] Ensure migrations are idempotent on repeated startup.
- [x] Add integration tests for opening a fresh temporary database.
- [x] Add integration tests for migration re-run behavior.

## Phase 4: Metadata Storage

- [x] Implement `internal/sqlite/metadata_store.go`.
- [x] Add methods to read and write `instance_metadata`.
- [x] Persist `instance_name`, `created_at`, and `scheduler_port`.
- [x] Add integration tests for metadata upsert and retrieval.

## Phase 5: Job Store

- [x] Implement `Create` in `internal/sqlite/jobs_store.go`.
- [x] Implement `GetByName`.
- [x] Implement `List`.
- [x] Implement `Update`.
- [x] Implement `DeleteByName`.
- [x] Implement `ListDue`.
- [x] Implement `UpdateNextRun`.
- [x] Implement `UpdateLastRunSummary`.
- [x] Add integration tests for job CRUD.
- [x] Add integration tests for unique job name enforcement.
- [x] Add integration tests for due job queries.

## Phase 6: Run Store

- [x] Implement `EnqueueManual` in `internal/sqlite/runs_store.go`.
- [x] Implement `EnqueueScheduled`.
- [x] Implement `ClaimPending`.
- [x] Implement `MarkRunning`.
- [x] Implement `MarkFinished`.
- [x] Implement `CancelPendingByJob`.
- [x] Implement `List`.
- [x] Implement `Get`.
- [x] Implement output row upsert for `job_run_outputs`.
- [x] Add integration tests for manual and scheduled run enqueue.
- [x] Add integration tests for run claiming and status transitions.
- [x] Add integration tests for output persistence.
- [x] Add integration tests for cascade delete from `jobs` to `job_runs` and `job_run_outputs`.

## Phase 7: Schedule Parsing

- [x] Implement parsing for `every <duration>` in `internal/schedule/parser.go`.
- [x] Implement parsing for `cron <expr>`.
- [x] Implement parsing for `after <duration>`.
- [x] Implement schedule normalization into `ScheduleKind` and expression fields.
- [x] Implement next-run calculation for interval schedules.
- [x] Implement next-run calculation for cron schedules.
- [x] Implement next-run calculation for one-time schedules.
- [x] Add table-driven tests for valid schedule strings.
- [x] Add table-driven tests for invalid schedule strings.
- [x] Add tests for next-run calculation edge cases.

## Phase 8: File Locking

- [x] Implement instance file locking in `internal/lock/filelock.go`.
- [x] Ensure lock acquisition fails clearly when the same instance is already running.
- [x] Ensure different instances can acquire independent locks.
- [x] Add tests for duplicate instance startup rejection.
- [x] Add tests for independent instance lock success.

## Phase 9: Daemon State and Serve Mode

- [x] Define the runtime state JSON structure.
- [x] Implement state file write and read helpers.
- [x] Implement the hidden internal `scheduler serve` command.
- [x] Re-exec the binary from `scheduler start` into serve mode.
- [x] Ensure the child process initializes paths, database, metadata,
  and lock in the correct order.
- [x] Remove the state file during graceful shutdown.
- [x] Add tests for state file lifecycle.

## Phase 10: Control API

- [x] Implement token generation for daemon instances.
- [x] Store the token in the runtime state file.
- [x] Implement `GET /v1/ping`.
- [x] Implement `GET /v1/scheduler`.
- [x] Implement `POST /v1/scheduler/shutdown`.
- [x] Reject requests with a missing or invalid `X-Jobs-Token`.
- [x] Add integration tests for control API success cases.
- [x] Add integration tests for authentication failures.

## Phase 11: Scheduler Start, Status, Stop, and Ping

- [x] Implement `scheduler start` parent flow and readiness polling.
- [x] Implement timeout and error reporting for failed startup.
- [x] Implement `scheduler status` using state file plus control API.
- [x] Classify status as `running`, `stale`, or `stopped`.
- [x] Implement `scheduler stop` with graceful shutdown verification.
- [x] Implement `scheduler ping` with correct exit codes.
- [x] Add CLI tests for required flags and output modes.
- [x] Add integration tests for start and stop flow.

## Phase 11.5: Windows Runtime Support

- [x] Define Windows-specific process spawning behavior for `scheduler start`.
- [x] Implement Windows-compatible detached/background serve launch.
- [x] Implement Windows-specific instance file locking semantics.
- [x] Ensure duplicate startup rejection works on Windows.
- [x] Ensure independent instances can run concurrently on Windows.
- [x] Verify state file lifecycle matches Unix behavior on Windows.
- [x] Verify `scheduler stop` can shut down a Windows daemon cleanly.
- [x] Add platform-gated tests for Windows lock behavior.
- [x] Add platform-gated tests for Windows start/stop lifecycle.
- [x] Add GitHub Actions multi-platform verification for Linux, macOS,
  and Windows.
- [x] Update architecture and implementation docs with Windows runtime notes.

## Phase 12: Executor

- [x] Implement shell command execution for Unix with `sh -lc`.
- [x] Implement shell command execution for Windows with `cmd /C`.
- [x] Capture stdout and stderr separately.
- [x] Enforce a `64 KiB` cap per output stream.
- [x] Mark truncation flags when output is cut off.
- [x] Return exit code and execution error details.
- [x] Add tests for successful execution.
- [x] Add tests for non-zero exit code handling.
- [x] Add tests for stdout and stderr truncation.

## Phase 13: Scheduler Loop

- [x] Implement the one-second polling loop in `internal/daemon/loop.go`.
- [x] Query due jobs using `enabled = 1` and `next_run_at <= now`.
- [x] Enqueue scheduled runs.
- [x] Recompute and persist `next_run_at` after scheduled enqueue.
- [x] For one-time schedules, disable the job after scheduled enqueue.
- [x] For one-time schedules, clear `next_run_at` after scheduled enqueue.
- [x] Claim pending runs for execution.
- [x] Mark claimed runs as running.
- [x] Execute claimed runs through the executor.
- [x] Persist final run state and outputs.
- [x] Update `jobs.last_run_at` and `jobs.last_run_status`.
- [x] Add loop tests with fake stores and fake executors.

## Phase 14: Concurrency Policies

- [x] Implement `forbid` behavior for manual triggers.
- [x] Implement `forbid` behavior for scheduled triggers.
- [x] Implement `queue` behavior.
- [x] Implement cancellation of older pending runs for `replace`.
- [x] Track in-memory execution handles for active runs.
- [x] Implement replacement of running jobs for `replace`.
- [x] Add tests for `forbid`, `queue`, and `replace`.
- [x] Add tests for scheduled skip plus next-run recalculation.

## Phase 15: Job Commands

- [x] Implement `job add`.
- [x] Validate required flags and parse schedules before persistence.
- [x] Compute initial `next_run_at` unless the job is disabled.
- [x] Compute initial `next_run_at` for enabled one-time schedules.
- [x] Implement `job list`.
- [x] Implement `job get`.
- [x] Implement `job update` with partial field updates.
- [x] Recompute `next_run_at` when schedule, timezone, or enabled state changes.
- [x] Recompute `next_run_at` correctly for one-time schedules on update and resume.
- [x] Implement `job delete`.
- [x] Implement `job pause`.
- [x] Implement `job resume`.
- [x] Implement `job run`.
- [x] Add CLI tests for all job command validation paths.
- [x] Add integration tests for job command behavior against a
  temporary instance database.

## Phase 16: Run Commands

- [x] Implement `run list` with `--job`, `--status`, and `--limit`.
- [x] Implement `run get`.
- [x] Include output truncation metadata in detailed run output.
- [x] Add CLI tests for run command validation.
- [x] Add integration tests for run list and run get behavior.

## Phase 17: Output Formatting and UX

- [x] Finalize human-readable table rendering.
- [x] Finalize JSON output payload shapes.
- [x] Ensure all help text uses `jobsd`.
- [x] Ensure all command examples use `jobsd`.
- [x] Add snapshot-like tests or stable string assertions for key command outputs.

## Phase 18: Version Command

- [x] Implement `version` command output.
- [x] Print version, commit, and build date.
- [x] Support both table and JSON output modes.
- [x] Add tests for version output formatting.

## Phase 19: End-to-End Tests

- [ ] Add an end-to-end test for `scheduler start -> job add -> job run
  -> run list -> run get -> scheduler stop`.
- [ ] Add an end-to-end test for automatic interval execution.
- [ ] Add an end-to-end test for automatic cron execution.
- [ ] Add an end-to-end test for one-time execution with automatic disable.
- [ ] Add an end-to-end test for duplicate start rejection on the same instance.

## Phase 20: Documentation and Release Readiness

- [ ] Create `README.md`.
- [ ] Document installation and build commands.
- [ ] Document the instance model and storage layout.
- [ ] Document the supported schedule grammar.
- [ ] Document common command workflows.
- [ ] Update `docs/v1/CONCEPT.md` examples to use `jobsd`.
- [ ] Update `docs/v1/CLI.md` examples to use `jobsd`.
- [ ] Verify that `docs/v1/ARCHITECTURE.md`, `docs/v1/SCHEMA.md`, and
  `docs/v1/IMPLEMENTATION_PLAN.md` still match the implementation.
- [ ] Add a simple release/build section with version injection guidance.

## Final Verification Checklist

- [ ] `go test ./...` passes.
- [ ] CLI tests, integration tests, and end-to-end tests all pass.
- [ ] Manual smoke test works on a clean local instance.
- [ ] No document still uses the old binary name `jobs`.
- [ ] The implementation matches the locked decisions in `docs/v1/IMPLEMENTATION_PLAN.md`.
