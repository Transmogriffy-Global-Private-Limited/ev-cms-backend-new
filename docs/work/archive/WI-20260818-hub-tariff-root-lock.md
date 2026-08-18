# WI-20260818-hub-tariff-root-lock

Status: Completed
Owner: Anubhab Dey
Collaborators: Codex
Started: 2026-08-18
Last updated: 2026-08-18

## Outcome

Made the source-only migration 44 database boundary serialize Hub publication
with same-Hub tariff topology mutation.

## Scope

- `db/migrations/000044_temporal_tariff_fallback.*.sql`
- `db/migration_test.go`
- source-state documentation and this work record

## Non-goals

- tariff semantics, pricing, GST, wallet, User App behavior, HAL/OCPP,
  deployment, database access, and runtime verification

## Claimed surfaces

- migration 44 Hub root-floor trigger and migration static coverage

## Dependencies and blockers

- Migration 44 remains source-only/unapplied according to `docs/PROJECT_STATE.md`.

## Contract impact

- None. This hardens the existing direct-DB concurrency invariant.

## Data and migration impact

- Amended source-only migration 44 only; it was not executed.

## Verification

- Passed the database-free migration static test, `go test -p 1 ./...`,
  `go vet ./...`, documentation verification, and `git diff --check`.
- PostgreSQL runtime/lifecycle/concurrency testing remains intentionally
  unperformed because no disposable `TEST_DATABASE_URL` was selected.

## Completion

- `guard_customer_visible_hub_tariff_root()` now acquires
  `tariff:<cpo_id>:hub:<hub_id>` before its root-floor query, identical to the
  Hub-target branch in `validate_temporal_tariff_target()`.
- The static test asserts the guard function, exact lock expression, and
  ordering before the root-floor check.
