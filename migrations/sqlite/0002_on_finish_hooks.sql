ALTER TABLE jobs ADD COLUMN on_finish_json TEXT;
ALTER TABLE jobs ADD COLUMN disable_inherited_on_finish INTEGER NOT NULL DEFAULT 0;

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

CREATE INDEX IF NOT EXISTS idx_run_hook_deliveries_run_id_attempt
    ON run_hook_deliveries(run_id, attempt);
