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

- [ ] Create the package directories under `internal/`.
- [ ] Add empty root command wiring in `internal/app/root.go`.
- [ ] Add placeholder command files for `scheduler`, `job`, `run`, and `version`.
- [ ] Add `internal/domain/job.go` with the `Job` type.
- [ ] Add `internal/domain/run.go` with the `Run` type.
- [ ] Add `internal/domain/types.go` with shared enums and helper types.
- [ ] Add `internal/output/printer.go` with table and JSON output scaffolding.

## Phase 2: Path and Runtime Layout

- [ ] Implement instance name validation in `internal/config/paths.go`.
- [ ] Implement data directory resolution for instance databases.
- [ ] Implement runtime directory resolution for lock and state files.
- [ ] Implement helper functions for database path, lock path, and state file path.
- [ ] Add table-driven tests for valid and invalid instance names.
- [ ] Add tests for XDG path resolution and fallback behavior.

## Phase 3: SQLite Foundation

- [ ] Add `migrations/sqlite/0001_init.sql`.
- [ ] Create `internal/sqlite/db.go` to open SQLite with the required pragmas.
- [ ] Create `internal/sqlite/migrate.go` to apply the initial migration.
- [ ] Ensure migrations are idempotent on repeated startup.
- [ ] Add integration tests for opening a fresh temporary database.
- [ ] Add integration tests for migration re-run behavior.

## Phase 4: Metadata Storage

- [ ] Implement `internal/sqlite/metadata_store.go`.
- [ ] Add methods to read and write `instance_metadata`.
- [ ] Persist `instance_name`, `created_at`, and `scheduler_port`.
- [ ] Add integration tests for metadata upsert and retrieval.

## Phase 5: Job Store

- [ ] Implement `Create` in `internal/sqlite/jobs_store.go`.
- [ ] Implement `GetByName`.
- [ ] Implement `List`.
- [ ] Implement `Update`.
- [ ] Implement `DeleteByName`.
- [ ] Implement `ListDue`.
- [ ] Implement `UpdateNextRun`.
- [ ] Implement `UpdateLastRunSummary`.
- [ ] Add integration tests for job CRUD.
- [ ] Add integration tests for unique job name enforcement.
- [ ] Add integration tests for due job queries.

## Phase 6: Run Store

- [ ] Implement `EnqueueManual` in `internal/sqlite/runs_store.go`.
- [ ] Implement `EnqueueScheduled`.
- [ ] Implement `ClaimPending`.
- [ ] Implement `MarkRunning`.
- [ ] Implement `MarkFinished`.
- [ ] Implement `CancelPendingByJob`.
- [ ] Implement `List`.
- [ ] Implement `Get`.
- [ ] Implement output row upsert for `job_run_outputs`.
- [ ] Add integration tests for manual and scheduled run enqueue.
- [ ] Add integration tests for run claiming and status transitions.
- [ ] Add integration tests for output persistence.
- [ ] Add integration tests for cascade delete from `jobs` to `job_runs` and `job_run_outputs`.

## Phase 7: Schedule Parsing

- [ ] Implement parsing for `every <duration>` in `internal/schedule/parser.go`.
- [ ] Implement parsing for `cron <expr>`.
- [ ] Implement parsing for `after <duration>`.
- [ ] Implement schedule normalization into `ScheduleKind` and expression fields.
- [ ] Implement next-run calculation for interval schedules.
- [ ] Implement next-run calculation for cron schedules.
- [ ] Implement next-run calculation for one-time schedules.
- [ ] Add table-driven tests for valid schedule strings.
- [ ] Add table-driven tests for invalid schedule strings.
- [ ] Add tests for next-run calculation edge cases.

## Phase 8: File Locking

- [ ] Implement instance file locking in `internal/lock/filelock.go`.
- [ ] Ensure lock acquisition fails clearly when the same instance is already running.
- [ ] Ensure different instances can acquire independent locks.
- [ ] Add tests for duplicate instance startup rejection.
- [ ] Add tests for independent instance lock success.

## Phase 9: Daemon State and Serve Mode

- [ ] Define the runtime state JSON structure.
- [ ] Implement state file write and read helpers.
- [ ] Implement the hidden internal `scheduler serve` command.
- [ ] Re-exec the binary from `scheduler start` into serve mode.
- [ ] Ensure the child process initializes paths, database, metadata,
  and lock in the correct order.
- [ ] Remove the state file during graceful shutdown.
- [ ] Add tests for state file lifecycle.

## Phase 10: Control API

- [ ] Implement token generation for daemon instances.
- [ ] Store the token in the runtime state file.
- [ ] Implement `GET /v1/ping`.
- [ ] Implement `GET /v1/scheduler`.
- [ ] Implement `POST /v1/scheduler/shutdown`.
- [ ] Reject requests with a missing or invalid `X-Jobs-Token`.
- [ ] Add integration tests for control API success cases.
- [ ] Add integration tests for authentication failures.

## Phase 11: Scheduler Start, Status, Stop, and Ping

- [ ] Implement `scheduler start` parent flow and readiness polling.
- [ ] Implement timeout and error reporting for failed startup.
- [ ] Implement `scheduler status` using state file plus control API.
- [ ] Classify status as `running`, `stale`, or `stopped`.
- [ ] Implement `scheduler stop` with graceful shutdown verification.
- [ ] Implement `scheduler ping` with correct exit codes.
- [ ] Add CLI tests for required flags and output modes.
- [ ] Add integration tests for start and stop flow.

## Phase 12: Executor

- [ ] Implement shell command execution for Unix with `sh -lc`.
- [ ] Implement shell command execution for Windows with `cmd /C`.
- [ ] Capture stdout and stderr separately.
- [ ] Enforce a `64 KiB` cap per output stream.
- [ ] Mark truncation flags when output is cut off.
- [ ] Return exit code and execution error details.
- [ ] Add tests for successful execution.
- [ ] Add tests for non-zero exit code handling.
- [ ] Add tests for stdout and stderr truncation.

## Phase 13: Scheduler Loop

- [ ] Implement the one-second polling loop in `internal/daemon/loop.go`.
- [ ] Query due jobs using `enabled = 1` and `next_run_at <= now`.
- [ ] Enqueue scheduled runs.
- [ ] Recompute and persist `next_run_at` after scheduled enqueue.
- [ ] Claim pending runs for execution.
- [ ] Mark claimed runs as running.
- [ ] Execute claimed runs through the executor.
- [ ] Persist final run state and outputs.
- [ ] Update `jobs.last_run_at` and `jobs.last_run_status`.
- [ ] Add loop tests with fake stores and fake executors.

## Phase 14: Concurrency Policies

- [ ] Implement `forbid` behavior for manual triggers.
- [ ] Implement `forbid` behavior for scheduled triggers.
- [ ] Implement `queue` behavior.
- [ ] Implement cancellation of older pending runs for `replace`.
- [ ] Track in-memory execution handles for active runs.
- [ ] Implement replacement of running jobs for `replace`.
- [ ] Add tests for `forbid`, `queue`, and `replace`.
- [ ] Add tests for scheduled skip plus next-run recalculation.

## Phase 15: Job Commands

- [ ] Implement `job add`.
- [ ] Validate required flags and parse schedules before persistence.
- [ ] Compute initial `next_run_at` unless the job is disabled.
- [ ] Implement `job list`.
- [ ] Implement `job get`.
- [ ] Implement `job update` with partial field updates.
- [ ] Recompute `next_run_at` when schedule, timezone, or enabled state changes.
- [ ] Implement `job delete`.
- [ ] Implement `job pause`.
- [ ] Implement `job resume`.
- [ ] Implement `job run`.
- [ ] Add CLI tests for all job command validation paths.
- [ ] Add integration tests for job command behavior against a
  temporary instance database.

## Phase 16: Run Commands

- [ ] Implement `run list` with `--job`, `--status`, and `--limit`.
- [ ] Implement `run get`.
- [ ] Include output truncation metadata in detailed run output.
- [ ] Add CLI tests for run command validation.
- [ ] Add integration tests for run list and run get behavior.

## Phase 17: Output Formatting and UX

- [ ] Finalize human-readable table rendering.
- [ ] Finalize JSON output payload shapes.
- [ ] Ensure all help text uses `jobsd`.
- [ ] Ensure all command examples use `jobsd`.
- [ ] Add snapshot-like tests or stable string assertions for key command outputs.

## Phase 18: Version Command

- [ ] Implement `version` command output.
- [ ] Print version, commit, and build date.
- [ ] Support both table and JSON output modes.
- [ ] Add tests for version output formatting.

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
