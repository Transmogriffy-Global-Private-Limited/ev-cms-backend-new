# WI-20260818-temporal-tariff-fallback

Status: Implemented — disposable PostgreSQL lifecycle/concurrency verification pending
Owner: Anubhab Dey
Collaborators: Codex; coordinated with the active CPO pricing work item
Started: 2026-08-18
Last updated: 2026-08-18

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — tariff/GST commercial correction and charging lifecycle

Detailed-plan reference: None

Issue/PR reference: None

## Outcome

Implemented deterministic temporal tariff fallback within each immutable tariff
target while retaining `UserGroup > Charger > Hub` scope precedence and the
published-Hub commercial floor.

## Scope

- active tariff temporal roles, topology admission, and PostgreSQL protection
- Hub/Charger/UserGroup create, PATCH, and delete tariff mutations
- Hub customer-visibility commercial-floor admission
- User App price and charging-admission resolution
- migrations, tests, OpenAPI, CPO/User App frontend handoffs, and project memory

## Non-goals

- HAL/OCPP behavior, GST ownership, wallet accounting rules, mutable existing
  charging snapshots, timers/workers, priority fields, deployment, or runtime
  database mutation

## Claimed surfaces

Released: `db/migrations/`, `src/commercial/`, `src/cpo/`,
`src/customerauth/`, tariff HTTP/OpenAPI/frontend/integration documentation.

## Dependencies and blockers

- Migration 44 replaces migration 15/37's no-overlap rule.
- The remaining verification requires an explicitly selected disposable
  `TEST_DATABASE_URL`; no migration was applied from this worktree.

## Contract impact

- Scoped tariff DELETE operations and a deterministic enabled temporal model.
- `is_active` means administratively enabled; effective policy is derived at a
  requested instant. Tariff dates do not configure the session-duration cutoff.

## Current state

- Source implementation, OpenAPI, frontend handoffs, static migration checks,
  focused Go checks, full `go test -p 1 ./...`, `go vet ./...`, and docs
  verification passed.
- No commit, push, deployment, service restart, or database mutation occurred.

## Verification

- Passed database-free temporal hierarchy and resolver tests, CPO mutation
  tests, migration static checks, route/OpenAPI coverage, full Go test/vet, and
  `scripts/verify-docs.ps1`.
- Guarded PostgreSQL lifecycle tests were compiled and skipped because
  `TEST_DATABASE_URL` is unset. They must be run against a disposable database
  before applying migration 44 anywhere.

## Handoff

- Preserve target precedence, Hub-owned GST, immutable commercial snapshots,
  and fail-closed resolution on malformed persisted topology.
- Next safe action: set an explicitly disposable `TEST_DATABASE_URL`, run the
  guarded CPO/customer lifecycle suite, validate migration up/down and
  concurrent same-target writes, then consider release separately.

## Completion

Source implementation complete; database lifecycle verification pending.
