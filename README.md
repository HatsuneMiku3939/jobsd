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

Install with Homebrew on macOS:

```bash
brew tap HatsuneMiku3939/homebrew-tap
brew install --cask jobsd
```

Install on Linux or Windows from GitHub Releases:

```text
https://github.com/HatsuneMiku3939/jobsd/releases
```

Install on Linux with native packages when available:

- `jobsd_<version>_x86_64.deb`
- `jobsd_<version>_arm64.deb`
- `jobsd-<version>-1.x86_64.rpm`
- `jobsd-<version>-1.aarch64.rpm`

You can also use the release archives:

- `jobsd_<version>_Linux_x86_64.tar.gz`
- `jobsd_<version>_Linux_arm64.tar.gz`
- `jobsd_<version>_Windows_x86_64.zip`
- `jobsd_<version>_Windows_arm64.zip`

Build from the current checkout:

```bash
make build
```

Check the installed version:

```bash
jobsd version
```

## Release

Pushing a tag that starts with `v` publishes a GitHub Release through
GitHub Actions and GoReleaser.

```bash
git tag v0.9.0
git push origin v0.9.0
```

Validate the release configuration locally before pushing a tag:

```bash
make test
make lint
make release-check
make release-snapshot
```

To update the Homebrew tap during release, add
`HOMEBREW_TAP_GITHUB_TOKEN` to the repository secrets with write access
to `HatsuneMiku3939/homebrew-tap`.

## Quick start

Start a scheduler instance:

```bash
jobsd scheduler start --instance dev
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

Configure an instance-level `on_finish` hook:

```bash
jobsd scheduler on-finish set \
  --instance dev \
  --config-json '{"type":"http","http":{"url":"http://127.0.0.1:8080/hooks/jobsd"}}'
```

Override the hook for one job:

```bash
jobsd job update \
  --instance dev \
  --name cleanup \
  --on-finish-config-json '{"type":"command","command":{"program":"echo","args":["hook"]}}'
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
jobsd scheduler start --instance dev
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

`on_finish` integration:

```bash
jobsd scheduler on-finish get --instance dev
jobsd scheduler on-finish set --instance dev --config-json '{"type":"http","http":{"url":"http://127.0.0.1:8080/hooks/jobsd"}}'
jobsd scheduler on-finish clear --instance dev
jobsd job add --instance dev --name cleanup --schedule "every 10m" --command "echo cleanup" --on-finish-config-json '{"type":"command","command":{"program":"echo"}}'
jobsd job update --instance dev --name cleanup --disable-inherited-on-finish
jobsd job update --instance dev --name cleanup --inherit-on-finish
jobsd job update --instance dev --name cleanup --clear-on-finish
```

`on_finish` delivery data:

- `command` hooks receive the payload JSON on `stdin`.
- `command` hooks also receive `JOBSD_EVENT`, `JOBSD_INSTANCE`, and `JOBSD_RUN_ID` in the environment.
- `http` hooks receive the same JSON payload in the `POST` body.
- `http` hooks always set `Content-Type: application/json` and then apply any configured custom headers.

Example payload:

```json
{
  "version": 1,
  "event": "run.finished",
  "instance": "dev",
  "job_name": "cleanup",
  "run_id": 11,
  "schedule": "every 10m",
  "command": "echo cleanup",
  "status": "succeeded",
  "exit_code": 0,
  "started_at": "2025-04-10T10:00:00Z",
  "finished_at": "2025-04-10T10:00:03Z",
  "duration_ms": 3000,
  "stdout_preview": "hello",
  "stderr_preview": "warn",
  "stdout_path": null,
  "stderr_path": null
}
```

Payload notes:

- `event` is currently always `run.finished`.
- `status` matches the finalized run status and is not rewritten when hook delivery fails.
- `stdout_preview` and `stderr_preview` are capped to `2048` bytes each.
- `stdout_path` and `stderr_path` are reserved for future use and are currently `null`.

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
- The scheduler control API binds to an auto-assigned `127.0.0.1:<port>`.
- The active control port is recorded in `state.json` and shown by
  `scheduler status` and `scheduler ping`.
- Unix-like systems execute job commands with `sh -lc`.
- Windows executes job commands with `cmd /C`.
- Output capture is stored in SQLite and capped at `64 KiB` per stream.
- `on_finish` supports one sink per scope in v1: `command` or `http`.
- Hook payloads are always JSON and use the same schema for both sink types.
- HTTP hooks are restricted to loopback targets such as `127.0.0.1` and `localhost`.
- Hook delivery failures are recorded separately and do not change the main job run result.

## Additional documentation

Design and internal reference documents are available in:

- `docs/v1/CONCEPT.md`
- `docs/v1/ARCHITECTURE.md`
- `docs/v1/SCHEMA.md`
- `docs/MANUAL_E2E_CHECKLIST.md`
