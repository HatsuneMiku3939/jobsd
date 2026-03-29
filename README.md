# jobsd

`jobsd` is a local, instance-oriented job scheduler that runs without
depending on `cron`.

Each scheduler instance is isolated by name and owns its own SQLite
database, runtime state, and execution history.

## Why jobsd

- Run scheduled jobs without editing system cron tables.
- Keep environments separate with explicit `--instance` targeting.
- Store job definitions and run history in SQLite.
- Manage the scheduler with a local CLI.

## Installation

Build the binary locally:

```bash
make build
```

Install from the current checkout:

```bash
go install ./cmd/jobsd
```

Check the installed version:

```bash
jobsd version
```

## Quick start

Start a scheduler instance:

```bash
jobsd scheduler start --instance dev --port 8080
```

Add a job:

```bash
jobsd job add \
  --instance dev \
  --name cleanup \
  --schedule "every 10m" \
  --command "echo cleanup"
```

List jobs:

```bash
jobsd job list --instance dev
```

Trigger a manual run:

```bash
jobsd job run --instance dev --name cleanup
```

Inspect run history:

```bash
jobsd run list --instance dev
jobsd run get --instance dev --run-id 1
```

Stop the scheduler:

```bash
jobsd scheduler stop --instance dev
```

## Schedule syntax

`jobsd` supports three schedule forms:

- `every <duration>` for recurring interval schedules
- `cron <five-field expr>` for cron schedules
- `after <duration>` for one-time schedules

Examples:

```bash
jobsd job add --instance dev --name cleanup --schedule "every 10m" --command "echo cleanup"
jobsd job add --instance dev --name report --schedule "cron 0 * * * *" --timezone UTC --command "echo report"
jobsd job add --instance dev --name bootstrap --schedule "after 30s" --command "echo bootstrap"
```

One-time schedules are disabled automatically after their scheduled run
is queued.

## Common commands

Scheduler management:

```bash
jobsd scheduler start --instance dev --port 8080
jobsd scheduler status --instance dev
jobsd scheduler ping --instance dev
jobsd scheduler stop --instance dev
```

Job management:

```bash
jobsd job add --instance dev --name cleanup --schedule "every 10m" --command "echo cleanup"
jobsd job list --instance dev
jobsd job get --instance dev --name cleanup
jobsd job update --instance dev --name cleanup --schedule "every 30m"
jobsd job pause --instance dev --name cleanup
jobsd job resume --instance dev --name cleanup
jobsd job delete --instance dev --name cleanup
```

Run inspection:

```bash
jobsd run list --instance dev
jobsd run get --instance dev --run-id 1
```

## Instance storage

Persistent data:

```text
~/.local/share/jobsd/instances/<instance>/jobs.db
```

Runtime files with `XDG_RUNTIME_DIR`:

```text
${XDG_RUNTIME_DIR}/jobsd/<instance>.lock
${XDG_RUNTIME_DIR}/jobsd/<instance>/state.json
```

Runtime fallback without `XDG_RUNTIME_DIR`:

```text
${TMPDIR:-/tmp}/jobsd-<uid>/<instance>.lock
${TMPDIR:-/tmp}/jobsd-<uid>/<instance>/state.json
```

Starting the same instance twice is rejected by the lock layer.

## Output and platform behavior

- `jobsd` supports `table` and `json` output modes through `--output`.
- The scheduler control API binds to `127.0.0.1:<port>`.
- Unix-like systems execute job commands with `sh -lc`.
- Windows executes job commands with `cmd /C`.
- Output capture is stored in SQLite and capped at `64 KiB` per stream.

## Additional documentation

Design and internal reference documents are available in:

- `docs/v1/CONCEPT.md`
- `docs/v1/ARCHITECTURE.md`
- `docs/v1/SCHEMA.md`
