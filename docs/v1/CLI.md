# jobs-cli Command Tree

## Design Goals

The CLI is instance-oriented.
Most commands target a specific scheduler instance through `--instance`.

The command tree should:

- make instance boundaries explicit
- keep scheduler lifecycle management separate from job management
- make common operational tasks easy to discover
- remain small enough for a first release

## Top-Level Command Structure

```text
jobs
├── scheduler
│   ├── start
│   ├── status
│   ├── stop
│   └── ping
├── job
│   ├── add
│   ├── list
│   ├── get
│   ├── update
│   ├── delete
│   ├── pause
│   ├── resume
│   └── run
├── run
│   ├── list
│   └── get
└── version
```

## Command Groups

### `jobs scheduler`

Manage the lifecycle and health of a scheduler instance.

#### `jobs scheduler start`

Start a scheduler daemon for a specific instance.

Example:

```bash
jobs scheduler start --instance dev --port 8080
```

Required flags:

- `--instance`
- `--port`

Behavior:

- resolves the instance data directory automatically
- opens or creates the instance database
- acquires the instance lock
- starts the scheduler loop

#### `jobs scheduler status`

Show the current state of a scheduler instance.

Example:

```bash
jobs scheduler status --instance dev
```

Required flags:

- `--instance`

Expected output:

- instance name
- status
- port
- process information if available
- database path

#### `jobs scheduler stop`

Stop a running scheduler instance.

Example:

```bash
jobs scheduler stop --instance dev
```

Required flags:

- `--instance`

Notes:

- this command should request graceful shutdown
- it should fail clearly if the instance is not running

#### `jobs scheduler ping`

Check whether a scheduler instance is reachable.

Example:

```bash
jobs scheduler ping --instance dev
```

Required flags:

- `--instance`

Notes:

- useful for scripts and health checks
- should return a machine-friendly status

### `jobs job`

Manage job definitions within one instance.

#### `jobs job add`

Create a new scheduled job.

Example:

```bash
jobs job add \
  --instance dev \
  --name cleanup \
  --schedule "every 10m" \
  --command "cleanup-temp-files"
```

Required flags:

- `--instance`
- `--name`
- `--schedule`
- `--command`

Optional flags:

- `--timezone`
- `--disabled`
- `--concurrency-policy`

#### `jobs job list`

List jobs for one instance.

Example:

```bash
jobs job list --instance dev
```

Required flags:

- `--instance`

Useful fields:

- job name
- enabled state
- schedule
- next run time
- last run time

#### `jobs job get`

Show the details of one job.

Example:

```bash
jobs job get --instance dev --name cleanup
```

Required flags:

- `--instance`
- `--name`

#### `jobs job update`

Update a job definition.

Example:

```bash
jobs job update \
  --instance dev \
  --name cleanup \
  --schedule "every 30m"
```

Required flags:

- `--instance`
- `--name`

Notes:

- only provided fields should be changed

#### `jobs job delete`

Delete a job definition.

Example:

```bash
jobs job delete --instance dev --name cleanup
```

Required flags:

- `--instance`
- `--name`

#### `jobs job pause`

Disable scheduled execution for a job without deleting it.

Example:

```bash
jobs job pause --instance dev --name cleanup
```

Required flags:

- `--instance`
- `--name`

#### `jobs job resume`

Re-enable scheduled execution for a paused job.

Example:

```bash
jobs job resume --instance dev --name cleanup
```

Required flags:

- `--instance`
- `--name`

#### `jobs job run`

Trigger a job immediately.

Example:

```bash
jobs job run --instance dev --name cleanup
```

Required flags:

- `--instance`
- `--name`

Notes:

- this command should create a run record
- it should not bypass normal execution tracking

### `jobs run`

Inspect execution history for one instance.

#### `jobs run list`

List recent job runs.

Example:

```bash
jobs run list --instance dev
```

Required flags:

- `--instance`

Optional flags:

- `--job`
- `--status`
- `--limit`

Useful fields:

- run ID
- job name
- status
- started time
- finished time
- duration

#### `jobs run get`

Show the details of one run.

Example:

```bash
jobs run get --instance dev --run-id 123
```

Required flags:

- `--instance`
- `--run-id`

Useful fields:

- run metadata
- exit code
- error message
- captured output summary

### `jobs version`

Print the CLI version.

Example:

```bash
jobs version
```

## Global Flag Direction

The CLI should keep global flags minimal.

Possible global flags:

- `--output`
- `--verbose`
- `--config`

`--instance` should remain a command-level flag for most commands
because it is part of the operational target, not global process
configuration.

## First Release Recommendation

The first release should implement this subset first:

```text
jobs scheduler start
jobs scheduler status
jobs scheduler stop
jobs job add
jobs job list
jobs job get
jobs job delete
jobs job pause
jobs job resume
jobs job run
jobs run list
jobs run get
jobs version
```

`jobs job update` and `jobs scheduler ping` can be added early, but they
are not strictly required for the minimum viable product.

## Naming Notes

Why `scheduler` instead of `instance`:

- `scheduler start` and `scheduler status` describe daemon lifecycle more clearly
- `--instance` still identifies the target runtime unit
- it avoids introducing a separate registry-oriented mental model

Why `run` instead of `history`:

- `run` is shorter and maps directly to execution records
- `run list` and `run get` are easy to remember

## Example Workflow

```bash
jobs scheduler start --instance dev --port 8080
jobs job add --instance dev --name cleanup --schedule "every 10m" --command "cleanup-temp-files"
jobs job list --instance dev
jobs job run --instance dev --name cleanup
jobs run list --instance dev
jobs scheduler stop --instance dev
```
