# jobsd Concept

## Overview

`jobsd` is a local job scheduling system that runs without `cron`.
It consists of:

- a scheduler daemon that runs in the background
- a CLI used to manage jobs for a specific scheduler instance

The project is designed around isolated scheduler instances instead of a
single shared daemon.

## Core Idea

Each scheduler instance is an independent runtime unit with:

- its own instance name
- its own port
- its own SQLite database
- its own runtime state files

Instances are explicitly managed by name.
There is no global control plane and no shared metadata database.

## Design Principles

### 1. Instance isolation

Every instance owns its own SQLite database.
Jobs, execution history, and scheduler state are stored only in that instance database.
One instance must not read or write another instance's persistent state.

### 2. Explicit targeting

The CLI always operates on a specific instance.
Users must pass `--instance` instead of relying on global discovery or
implicit defaults.

### 3. Automatic storage layout

The database path is created automatically from the instance name.
Users should not need to manually create or register database files for normal usage.

### 4. OS-level single ownership

The same instance name must not be started more than once at the same time.
This constraint is enforced with an OS-level file lock based on the instance name.
SQLite is not responsible for process ownership control.

## Runtime Model

An instance is started with an instance name and a port.
When the daemon starts, it:

1. resolves the instance-specific data directory
2. acquires the instance lock file
3. opens or creates the instance SQLite database
4. starts the scheduler loop
5. exposes management endpoints on the configured port

If the lock for the same instance name is already held, startup must fail.

## Storage Model

Each instance uses its own automatically managed paths.

Example layout:

- persistent data: `~/.local/share/jobsd/instances/<instance>/jobs.db`
- lock file: `${XDG_RUNTIME_DIR}/jobsd/<instance>.lock`
- runtime files: `${XDG_RUNTIME_DIR}/jobsd/<instance>/`
- runtime fallback without `XDG_RUNTIME_DIR`: `${TMPDIR:-/tmp}/jobsd-<uid>/<instance>/`

Persistent data and runtime state must be separated.

## CLI Philosophy

The CLI is instance-oriented.
Commands should require the target instance explicitly.

Examples:

```bash
jobsd scheduler start --instance dev --port 8080
jobsd scheduler status --instance dev
jobsd job add --instance dev --name cleanup --schedule "every 10m" --command "..."
jobsd job list --instance dev
jobsd job run --instance dev --name cleanup
jobsd run list --instance dev
```

The CLI should not depend on a global instance registry for normal operation.

## Scope of the First Version

The first version should focus on:

- starting a scheduler instance
- preventing duplicate startup for the same instance name
- automatically creating the instance database
- registering and listing jobs
- running jobs on a schedule
- triggering jobs manually
- storing execution history

## Non-Goals for the First Version

The first version does not need to include:

- a shared multi-instance control database
- automatic global instance discovery
- distributed scheduling across machines
- high-availability clustering
- a web UI

## Product Positioning

`jobsd` can be described as:

> A local multi-instance job scheduler with isolated SQLite storage per instance.

It is not just a `cron` replacement.
It is a small, stateful, instance-scoped job runtime for local environments.
