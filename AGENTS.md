# AGENTS.md

## Purpose

This file gives project-specific guidance to AI coding agents working in
this repository.

Use it together with the current codebase, tests, and the remaining
design documents in `docs/v1`.

## Project snapshot

- Project name: `jobsd`
- Module path: `github.com/hatsunemiku3939/jobsd`
- Language: Go
- Binary entrypoint: `cmd/jobsd`
- Scheduler model: local, instance-oriented, one SQLite database per instance
- SQLite driver: `modernc.org/sqlite`
- Logging: `log/slog`

## Source of truth

When making decisions, use these sources in order:

1. The current implementation and tests
2. `docs/v1/CONCEPT.md`
3. `docs/v1/ARCHITECTURE.md`
4. `docs/v1/SCHEMA.md`

Do not rely on deleted planning documents. If the code and old design
intent differ, prefer the current implementation unless the user asks for
an intentional behavioral change.

## Documentation roles

- `README.md` is for end users.
- `docs/v1/*.md` contains internal product and design reference material.
- Keep internal implementation detail out of `README.md` unless it helps a
  normal user install, run, or operate `jobsd`.
- Update documentation whenever user-visible behavior, commands, storage
  layout, or supported schedules change.

## Repository map

- `cmd/jobsd`: binary entrypoint and end-to-end or lifecycle tests
- `internal/app`: Cobra command wiring and CLI handlers
- `internal/config`: path and instance resolution
- `internal/daemon`: scheduler runtime, control API, executor, loop
- `internal/domain`: shared domain types
- `internal/lock`: instance locking
- `internal/output`: table and JSON output helpers
- `internal/schedule`: schedule parsing and next-run calculation
- `internal/sqlite`: database setup, migrations, and stores
- `migrations/sqlite`: SQL migrations
- `version`: build metadata helpers

## Working agreement

- Keep the CLI instance-oriented. Most operations should require
  `--instance`.
- Preserve one-binary behavior. Public commands and daemon behavior should
  continue to ship through `jobsd`.
- Keep scheduler state isolated per instance.
- Avoid introducing CGO requirements unless the user explicitly asks for
  that tradeoff.
- Prefer small, focused changes that match the existing package
  boundaries.

## Code guidelines

- Write code and comments in English.
- Prefer clear, small functions over clever abstractions.
- Use `log/slog` for logging.
- Follow existing enum names, command names, and JSON field shapes unless
  the user asks for a breaking change.
- Keep SQL in `internal/sqlite` and keep raw SQL out of CLI wiring.
- Preserve cross-platform behavior. This project supports Unix-like
  systems and Windows.
- When adding platform-specific logic, follow the existing split-file
  pattern such as `_unix.go`, `_windows.go`, and `_other.go`.

## Testing expectations

- Add or update tests for every behavior change.
- Prefer table-driven tests where they fit naturally.
- Run `make test` after every code change.
- Run `make lint` before handing off or committing.
- If a change affects platform-specific behavior, update or add the
  relevant platform-gated tests.
- Prefer stable automated tests over broad but flaky end-to-end coverage.
- The end-to-end scheduler lifecycle tests under `cmd/jobsd` are opt-in
  and require the `e2e` build tag.
- Use `docs/MANUAL_E2E_CHECKLIST.md` when real process-level manual
  verification is needed.

Current repository commands:

```bash
make build
make test
make lint
go test -tags=e2e ./cmd/jobsd
```

## Change checklist

Before finishing work, make sure that:

- code changes include relevant tests
- user-facing documentation is updated when needed
- `make test` passes
- `make lint` passes
- no unrelated files were reverted or reformatted without a reason

## Common project-specific pitfalls

- Do not treat `README.md` as an internal design document.
- Do not assume Linux-only behavior; Windows behavior still matters even
  when it is not covered by the default automated test suite.
- Do not bypass the scheduler execution path for manual runs unless the
  user explicitly requests a design change.
- Do not break instance isolation by introducing shared mutable state
  across instances.

## When in doubt

- Read the nearest tests first.
- Follow the existing package structure.
- Choose the smallest change that keeps behavior explicit and testable.
