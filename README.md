# jobsd

## Project overview

`jobsd` is a local, instance-oriented job scheduler that runs without
depending on `cron`.

Each scheduler instance owns:

- its own SQLite database
- its own runtime state files
- its own lock file
- its own loopback control API

The CLI always targets a specific instance with `--instance`.

## Installation and local build

Build the binary locally:

```bash
make build
```

Install from the current checkout:

```bash
go install ./cmd/jobsd
```

Run the full test suite:

```bash
make test
```

Run lint checks:

```bash
make lint
```

## Build metadata / version injection

`jobsd version` prints the build version, commit, and build date.

Build with explicit metadata:

```bash
go build -ldflags "-X main.version=v1.0.0 -X main.commit=abc1234 -X main.buildDate=2025-03-29T00:00:00Z" -o ./bin/jobsd ./cmd/jobsd
```

If no values are injected, the binary falls back to the package version
and uses `unknown` for missing commit or build date values.

## Instance model and storage layout

Each instance is isolated by name. Starting the same instance twice is
rejected by the lock layer.

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

The state file includes:

- `instance`
- `pid`
- `port`
- `token`
- `db_path`
- `started_at`
- `version`

## Schedule grammar

Supported schedule forms:

- `every <duration>` for recurring interval schedules
- `cron <five-field expr>` for cron schedules
- `after <duration>` for one-time schedules

Examples:

```bash
jobsd job add --instance dev --name cleanup --schedule "every 10m" --command "echo cleanup"
jobsd job add --instance dev --name report --schedule "cron 0 * * * *" --timezone UTC --command "echo report"
jobsd job add --instance dev --name bootstrap --schedule "after 30s" --command "echo bootstrap"
```

One-time schedules run once through the normal scheduler path and are
disabled automatically after the scheduled execution is queued.

## Common workflows

Start a scheduler:

```bash
jobsd scheduler start --instance dev --port 8080
jobsd scheduler status --instance dev
jobsd scheduler ping --instance dev
```

Create and inspect jobs:

```bash
jobsd job add --instance dev --name cleanup --schedule "every 10m" --command "echo cleanup"
jobsd job list --instance dev
jobsd job get --instance dev --name cleanup
jobsd job pause --instance dev --name cleanup
jobsd job resume --instance dev --name cleanup
```

Trigger a manual run and inspect history:

```bash
jobsd job run --instance dev --name cleanup
jobsd run list --instance dev
jobsd run get --instance dev --run-id 1
```

Print version information:

```bash
jobsd version
jobsd --output json version
```

Stop a scheduler:

```bash
jobsd scheduler stop --instance dev
```

## Testing and linting

Repository commands:

```bash
make test
make lint
```

The test suite includes:

- unit tests
- integration tests against temporary SQLite databases
- end-to-end CLI tests that build and execute the real `jobsd` binary
- Windows lifecycle verification for detached daemon behavior

## Platform notes

- Unix-like systems execute job commands with `sh -lc`.
- Windows executes job commands with `cmd /C`.
- The scheduler control API binds to `127.0.0.1:<port>`.
- Output capture is stored in SQLite and capped at `64 KiB` per stream.
