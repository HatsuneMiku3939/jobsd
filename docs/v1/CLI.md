# jobsd Command Tree

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
jobsd
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

### `jobsd scheduler`

Manage the lifecycle and health of a scheduler instance.

#### `jobsd scheduler start`

Start a scheduler daemon for a specific instance.

Example:

```bash
jobsd scheduler start --instance dev --port 8080
```

Required flags:

- `--instance`
- `--port`

Behavior:

- resolves the instance data directory automatically
- opens or creates the instance database
- acquires the instance lock
- starts the scheduler loop

#### `jobsd scheduler status`

Show the current state of a scheduler instance.

Example:

```bash
jobsd scheduler status --instance dev
```

Required flags:

- `--instance`

Expected output:

- instance name
- status
- port
- process information if available
- database path

Status values:

- `running`
- `stale`
- `stopped`

#### `jobsd scheduler stop`

Stop a running scheduler instance.

Example:

```bash
jobsd scheduler stop --instance dev
```

Required flags:

- `--instance`

Notes:

- this command should request graceful shutdown
- it should fail clearly if the instance is not running

#### `jobsd scheduler ping`

Check whether a scheduler instance is reachable.

Example:

```bash
jobsd scheduler ping --instance dev
```

Required flags:

- `--instance`

Notes:

- useful for scripts and health checks
- should return a machine-friendly status

### `jobsd job`

Manage job definitions within one instance.

#### `jobsd job add`

Create a new scheduled job.

Example:

```bash
jobsd job add \
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

#### `jobsd job list`

List jobs for one instance.

Example:

```bash
jobsd job list --instance dev
```

Required flags:

- `--instance`

Useful fields:

- job name
- enabled state
- schedule
- next run time
- last run time

#### `jobsd job get`

Show the details of one job.

Example:

```bash
jobsd job get --instance dev --name cleanup
```

Required flags:

- `--instance`
- `--name`

#### `jobsd job update`

Update a job definition.

Example:

```bash
jobsd job update \
  --instance dev \
  --name cleanup \
  --new-name cleanup-nightly \
  --schedule "every 30m" \
  --timezone UTC \
  --concurrency-policy queue
```

Required flags:

- `--instance`
- `--name`

Notes:

- only provided fields should be changed
- `--new-name` renames the job while `--name` remains the lookup key
- `--enabled` and `--disabled` are mutually exclusive
- supported optional update flags are:
  - `--new-name`
  - `--command`
  - `--schedule`
  - `--timezone`
  - `--concurrency-policy`
  - `--enabled`
  - `--disabled`

#### `jobsd job delete`

Delete a job definition.

Example:

```bash
jobsd job delete --instance dev --name cleanup
```

Required flags:

- `--instance`
- `--name`

#### `jobsd job pause`

Disable scheduled execution for a job without deleting it.

Example:

```bash
jobsd job pause --instance dev --name cleanup
```

Required flags:

- `--instance`
- `--name`

#### `jobsd job resume`

Re-enable scheduled execution for a paused job.

Example:

```bash
jobsd job resume --instance dev --name cleanup
```

Required flags:

- `--instance`
- `--name`

#### `jobsd job run`

Trigger a job immediately.

Example:

```bash
jobsd job run --instance dev --name cleanup
```

Required flags:

- `--instance`
- `--name`

Notes:

- this command should create a run record
- it should not bypass normal execution tracking

### `jobsd run`

Inspect execution history for one instance.

#### `jobsd run list`

List recent job runs.

Example:

```bash
jobsd run list --instance dev
```

Required flags:

- `--instance`

Optional flags:

- `--job`
- `--status`
- `--limit`

Notes:

- `--limit` defaults to `20`

Useful fields:

- run ID
- job name
- status
- started time
- finished time
- duration

#### `jobsd run get`

Show the details of one run.

Example:

```bash
jobsd run get --instance dev --run-id 123
```

Required flags:

- `--instance`
- `--run-id`

Useful fields:

- run metadata
- exit code
- error message
- captured output summary
- stdout and stderr truncation metadata

### `jobsd version`

Print the CLI version.

Example:

```bash
jobsd version
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
jobsd scheduler start
jobsd scheduler status
jobsd scheduler stop
jobsd job add
jobsd job list
jobsd job get
jobsd job delete
jobsd job pause
jobsd job resume
jobsd job run
jobsd run list
jobsd run get
jobsd version
```

`jobsd job update` and `jobsd scheduler ping` can be added early, but they
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
jobsd scheduler start --instance dev --port 8080
jobsd job add --instance dev --name cleanup --schedule "every 10m" --command "cleanup-temp-files"
jobsd job list --instance dev
jobsd job run --instance dev --name cleanup
jobsd run list --instance dev
jobsd scheduler stop --instance dev
```
