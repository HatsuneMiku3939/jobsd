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
    next_run_at TEXT,
    last_run_at TEXT,
    last_run_status TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(name),
    CHECK(enabled IN (0, 1)),
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
