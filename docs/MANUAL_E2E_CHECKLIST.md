# Manual E2E Checklist

This checklist is for manual end-to-end verification of `jobsd`.

Use it when changing daemon lifecycle behavior, scheduler timing,
instance isolation, platform-specific process handling, or user-facing
CLI flows.

## Why manual E2E

`jobsd` depends on real processes, ports, timing, files, and operating
system behavior. Those paths are valuable to verify, but they are also
more likely to become flaky in automated CI.

The default automated test suite should stay stable and deterministic.
Use this checklist when a real end-to-end verification is needed.

Windows lifecycle end-to-end coverage is intentionally excluded from CI.
Run the Windows-specific lifecycle checks in this document manually when
those paths change.

## Preparation

- Build the binary with `make build`.
- Choose a fresh instance name for each run, such as `manual-e2e-1`.
- Use isolated data and runtime directories so existing local instances do
  not interfere.

Example setup on Unix-like systems:

```bash
export XDG_DATA_HOME="$(mktemp -d)"
export XDG_RUNTIME_DIR="$(mktemp -d)"
INSTANCE="manual-e2e-1"
./bin/jobsd version
```

Example setup on Windows PowerShell:

```powershell
$env:XDG_DATA_HOME = Join-Path $env:TEMP ("jobsd-data-" + [guid]::NewGuid())
$env:XDG_RUNTIME_DIR = Join-Path $env:TEMP ("jobsd-runtime-" + [guid]::NewGuid())
$INSTANCE = "manual-e2e-1"
.\bin\jobsd.exe version
```

## Checklist

### 1. Scheduler start

- Run `jobsd scheduler start --instance <instance>`.
- Confirm the command reports `running`.
- Confirm `jobsd scheduler status --instance <instance>` reports
  `running`.
- Confirm `jobsd scheduler ping --instance <instance>` succeeds.
- Confirm the reported port is non-zero and matches the value in the
  runtime `state.json` file.

### 2. Manual run workflow

- Add a job with a long interval:

```bash
jobsd job add --instance "$INSTANCE" --name manual-check --schedule "every 1h" --timezone UTC --command "printf 'manual-ok'"
```

- Run `jobsd job run --instance <instance> --name manual-check`.
- Confirm `jobsd run list --instance <instance> --job manual-check`
  shows a recent run.
- Confirm `jobsd run get --instance <instance> --run-id <id>` reaches
  `succeeded`.
- Confirm stdout is `manual-ok`.

### 3. Scheduled interval workflow

- Add an interval job:

```bash
jobsd job add --instance "$INSTANCE" --name interval-check --schedule "every 2s" --timezone UTC --command "printf 'interval-ok'"
```

- Wait for at least one scheduled run.
- Confirm `jobsd run list --instance <instance> --job interval-check`
  shows a `schedule` trigger.
- Confirm the latest run finishes with `succeeded`.

### 4. One-time schedule behavior

- Add a one-time job:

```bash
jobsd job add --instance "$INSTANCE" --name once-check --schedule "after 2s" --timezone UTC --command "printf 'once-ok'"
```

- Wait for the scheduled run to finish.
- Confirm `jobsd job get --instance <instance> --name once-check`
  shows `enabled = false`.
- Confirm `next_run_at` is empty.
- Confirm only one scheduled run was created.

### 5. Pause and resume

- Pause `interval-check` with
  `jobsd job pause --instance <instance> --name interval-check`.
- Confirm the job shows `enabled = false`.
- Wait a few seconds and confirm no new scheduled run is created.
- Resume it with
  `jobsd job resume --instance <instance> --name interval-check`.
- Confirm scheduled execution resumes.

### 6. Duplicate start rejection

- While the scheduler is still running, run another
  `jobsd scheduler start --instance <instance>`.
- Confirm the command fails with an `already running` style error.

### 7. Shutdown cleanup

- Run `jobsd scheduler stop --instance <instance>`.
- Confirm the command reports `stopped`.
- Confirm `jobsd scheduler status --instance <instance>` reports
  `stopped`.
- Confirm the runtime `state.json` file is removed.

## Optional platform checks

### Windows

- Repeat scheduler start and stop once more with a second instance name.
- Confirm two different instances can run at the same time.
- Confirm stopping one instance does not affect the other.

### Unix-like systems

- Verify runtime files are created under `XDG_RUNTIME_DIR`.
- Verify the database file is created under `XDG_DATA_HOME`.

## Cleanup

- Stop any remaining instances started during the check.
- Remove temporary `XDG_DATA_HOME` and `XDG_RUNTIME_DIR` directories if
  you created them manually.

## When to use this checklist

Run this checklist when changes touch:

- `cmd/jobsd`
- `internal/app`
- `internal/daemon`
- `internal/lock`
- `internal/config`
- cross-platform process behavior
- scheduler timing or lifecycle behavior
