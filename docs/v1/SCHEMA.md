# jobsd SQLite Schema

## Scope

This schema describes the SQLite database owned by one scheduler instance.
There is no shared database across instances.

Each instance database stores:

- job definitions
- pending and completed job runs
- lightweight scheduler metadata needed for local operation

## Design Goals

The schema should:

- remain small and easy to migrate
- support scheduled and manual runs
- keep job definitions separate from execution history
- make common scheduler queries fast
- avoid cross-instance dependencies

## Proposed Tables

```text
schema_migrations
jobs
job_runs
job_run_outputs
run_hook_deliveries
instance_metadata
```

## `schema_migrations`

Track applied database migrations.

Recommended columns:

- `version` INTEGER PRIMARY KEY
- `applied_at` TEXT NOT NULL

Notes:

- keep this table simple
- use UTC RFC3339 timestamps

## `jobs`

Store job definitions for the instance.

Recommended columns:

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `name` TEXT NOT NULL
- `command` TEXT NOT NULL
- `schedule_kind` TEXT NOT NULL
- `schedule_expr` TEXT NOT NULL
- `timezone` TEXT NOT NULL DEFAULT 'Local'
- `enabled` INTEGER NOT NULL DEFAULT 1
- `concurrency_policy` TEXT NOT NULL DEFAULT 'forbid'
- `on_finish_json` TEXT
- `disable_inherited_on_finish` INTEGER NOT NULL DEFAULT 0
- `next_run_at` TEXT
- `last_run_at` TEXT
- `last_run_status` TEXT
- `created_at` TEXT NOT NULL
- `updated_at` TEXT NOT NULL

Constraints:

- `UNIQUE(name)`
- `CHECK(enabled IN (0, 1))`
- `CHECK(disable_inherited_on_finish IN (0, 1))`
- `CHECK(concurrency_policy IN ('forbid', 'queue', 'replace'))`

Column intent:

- `schedule_kind`: a stable internal type such as `interval` or `cron`
- `schedule_expr`: the user-facing or normalized schedule expression
- `next_run_at`: next calculated execution time in UTC
- `last_run_status`: a denormalized summary field for list views
- `on_finish_json`: optional JSON hook config for a job-level override
- `disable_inherited_on_finish`: when `1`, suppress the instance default hook for this job

## `job_runs`

Store scheduled and manual run records.

Recommended columns:

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `job_id` INTEGER NOT NULL
- `trigger_type` TEXT NOT NULL
- `status` TEXT NOT NULL
- `scheduled_for` TEXT
- `queued_at` TEXT NOT NULL
- `started_at` TEXT
- `finished_at` TEXT
- `exit_code` INTEGER
- `error_message` TEXT
- `runner_id` TEXT

Constraints:

- `FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE`
- `CHECK(trigger_type IN ('schedule', 'manual'))`
- `CHECK(status IN ('pending', 'running', 'succeeded', 'failed', 'canceled'))`

Column intent:

- `trigger_type`: whether the run came from the scheduler or the CLI
- `status`: also acts as a lightweight queue state
- `scheduled_for`: original due time for scheduled runs
- `queued_at`: when the run record was created
- `runner_id`: optional marker for the process currently handling the run

## `job_run_outputs`

Store captured output for one run.

Recommended columns:

- `run_id` INTEGER PRIMARY KEY
- `stdout_text` TEXT NOT NULL DEFAULT ''
- `stderr_text` TEXT NOT NULL DEFAULT ''
- `stdout_truncated` INTEGER NOT NULL DEFAULT 0
- `stderr_truncated` INTEGER NOT NULL DEFAULT 0
- `updated_at` TEXT NOT NULL

Constraints:

- `FOREIGN KEY(run_id) REFERENCES job_runs(id) ON DELETE CASCADE`
- `CHECK(stdout_truncated IN (0, 1))`
- `CHECK(stderr_truncated IN (0, 1))`

Notes:

- this table should remain optional at runtime
- output should be capped to avoid uncontrolled database growth
- if output retention becomes large, move full logs to files later

## `run_hook_deliveries`

Store observability records for each `on_finish` delivery attempt.

Recommended columns:

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `run_id` INTEGER NOT NULL
- `event` TEXT NOT NULL
- `sink_type` TEXT NOT NULL
- `attempt` INTEGER NOT NULL
- `status` TEXT NOT NULL
- `http_status_code` INTEGER
- `error_message` TEXT
- `started_at` TEXT NOT NULL
- `finished_at` TEXT NOT NULL

Constraints:

- `FOREIGN KEY(run_id) REFERENCES job_runs(id) ON DELETE CASCADE`
- `CHECK(sink_type IN ('command', 'http'))`
- `CHECK(status IN ('succeeded', 'failed', 'timed_out'))`

Column intent:

- `event`: v1 always stores `run.finished`
- `sink_type`: delivery target type that was attempted
- `attempt`: 1-based attempt number for one finalized run event
- `http_status_code`: optional response code for HTTP hooks
- `error_message`: delivery-specific failure detail that does not affect the main run result

## `instance_metadata`

Store instance-local metadata that belongs in the database.

Recommended columns:

- `key` TEXT PRIMARY KEY
- `value` TEXT NOT NULL
- `updated_at` TEXT NOT NULL

Suggested keys:

- `instance_name`
- `created_at`
- `scheduler_port`
- `on_finish_json`

Notes:

- do not store lock ownership here
- do not depend on this table for runtime exclusivity
- `scheduler_port` stores the most recent loopback control port selected
  at scheduler startup
- `on_finish_json` stores the instance-level default hook config when set

## Recommended Indexes

```sql
CREATE UNIQUE INDEX idx_jobs_name ON jobs(name);
CREATE INDEX idx_jobs_enabled_next_run_at ON jobs(enabled, next_run_at);
CREATE INDEX idx_job_runs_job_id_queued_at ON job_runs(job_id, queued_at DESC);
CREATE INDEX idx_job_runs_status_queued_at ON job_runs(status, queued_at ASC);
CREATE INDEX idx_job_runs_scheduled_for ON job_runs(scheduled_for);
CREATE INDEX idx_run_hook_deliveries_run_id_attempt ON run_hook_deliveries(run_id, attempt);
```

These indexes optimize:

- finding the next due jobs
- listing recent runs for a job
- fetching pending runs for execution

## Initial SQL Draft

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    command TEXT NOT NULL,
    schedule_kind TEXT NOT NULL,
    schedule_expr TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'Local',
    enabled INTEGER NOT NULL DEFAULT 1,
    concurrency_policy TEXT NOT NULL DEFAULT 'forbid',
    on_finish_json TEXT,
    disable_inherited_on_finish INTEGER NOT NULL DEFAULT 0,
    next_run_at TEXT,
    last_run_at TEXT,
    last_run_status TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(name),
    CHECK(enabled IN (0, 1)),
    CHECK(disable_inherited_on_finish IN (0, 1)),
    CHECK(concurrency_policy IN ('forbid', 'queue', 'replace'))
);

CREATE TABLE IF NOT EXISTS job_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL,
    trigger_type TEXT NOT NULL,
    status TEXT NOT NULL,
    scheduled_for TEXT,
    queued_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    exit_code INTEGER,
    error_message TEXT,
    runner_id TEXT,
    FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE,
    CHECK(trigger_type IN ('schedule', 'manual')),
    CHECK(status IN ('pending', 'running', 'succeeded', 'failed', 'canceled'))
);

CREATE TABLE IF NOT EXISTS job_run_outputs (
    run_id INTEGER PRIMARY KEY,
    stdout_text TEXT NOT NULL DEFAULT '',
    stderr_text TEXT NOT NULL DEFAULT '',
    stdout_truncated INTEGER NOT NULL DEFAULT 0,
    stderr_truncated INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(run_id) REFERENCES job_runs(id) ON DELETE CASCADE,
    CHECK(stdout_truncated IN (0, 1)),
    CHECK(stderr_truncated IN (0, 1))
);

CREATE TABLE IF NOT EXISTS run_hook_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL,
    event TEXT NOT NULL,
    sink_type TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    status TEXT NOT NULL,
    http_status_code INTEGER,
    error_message TEXT,
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL,
    FOREIGN KEY(run_id) REFERENCES job_runs(id) ON DELETE CASCADE,
    CHECK(sink_type IN ('command', 'http')),
    CHECK(status IN ('succeeded', 'failed', 'timed_out'))
);

CREATE TABLE IF NOT EXISTS instance_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_name ON jobs(name);
CREATE INDEX IF NOT EXISTS idx_jobs_enabled_next_run_at ON jobs(enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_job_runs_job_id_queued_at ON job_runs(job_id, queued_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_runs_status_queued_at ON job_runs(status, queued_at ASC);
CREATE INDEX IF NOT EXISTS idx_job_runs_scheduled_for ON job_runs(scheduled_for);
CREATE INDEX IF NOT EXISTS idx_run_hook_deliveries_run_id_attempt ON run_hook_deliveries(run_id, attempt);
```

## Operational Notes

### Time handling

Store timestamps as UTC RFC3339 strings.
Convert to local time only at the CLI presentation layer.

### Job scheduling

The scheduler should query `jobs.enabled = 1` and `jobs.next_run_at <= now`.
After queuing a scheduled run, it should recalculate and update the next run time.

### Manual execution

`jobsd job run` should insert a `job_runs` row with:

- `trigger_type = 'manual'`
- `status = 'pending'`
- `queued_at = now`

The scheduler loop then picks it up through the same execution path.

### Output retention

For the first version, storing capped stdout and stderr in SQLite is acceptable.
The implementation should define a maximum size per run to keep the database healthy.

## First Version Recommendation

For the initial release, this subset is enough:

- `schema_migrations`
- `jobs`
- `job_runs`
- `job_run_outputs`
- `run_hook_deliveries`

`instance_metadata` is useful, but not strictly required for the minimum
viable product.
