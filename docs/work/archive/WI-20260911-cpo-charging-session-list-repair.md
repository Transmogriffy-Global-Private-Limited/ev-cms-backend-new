# WI-20260911-cpo-charging-session-list-repair

Status: Verified (source only)
Owner: Codex
Collaborators: None
Started: 2026-09-11
Last updated: 2026-09-11

Development-plan reference: Phase 5 charging-session projection
Detailed-plan reference: None; emergency production correction
Issue/PR reference: None

## Outcome

Repaired the CPO charging-session list query contract so every documented
filter, sort, cursor, and active-session usage projection has matching HTTP,
service, SQL, and response semantics.

## Scope

- `src/cpo` charging-session list parser, typed query, validation, repository,
  response pagination, and focused/PostgreSQL-gated tests.
- CPO/OpenAPI/project-state documentation for the corrected read contract.

## Non-goals

- HAL, migrations, invoices, CPO login, TriggerMessage, operation recovery,
  deployment, or runtime database changes.

## Claimed surfaces

- `src/cpo/router.go`, `schemas.go`, `service.go`, `repository.go`
- CPO charging-session list tests
- CPO/OpenAPI/project-memory documentation

## Dependencies and blockers

- `TEST_DATABASE_URL` is unavailable. The disposable PostgreSQL repository
  test is present and correctly skips, but its live SQL execution remains
  unverified.

## Contract impact

- Restored legacy newest-first `before` pagination, with optional `before_id`
  UUID tie-breaking, exactly separate from generic pagination.
- Generic cursors are complete typed pairs; duration page chains return and
  require a fixed `as_of`; an open `end_time` cursor is the literal `null`.
- Active list `total_kwh`, SQL usage filters, and usage ordering all use the
  same latest durable meter delta for open live statuses.

## Data and migration impact

None.

## Current state

Source verification is complete. No deployment, rehost, migration, HAL change,
or runtime database action has occurred.

## Verification

- Focused parser/service tests: passed.
- PostgreSQL repository traversal/filter test: skipped because
  `TEST_DATABASE_URL` is unset.
- OpenAPI runtime route coverage and documentation verification: passed.
- `go test -p 1 ./...`, `go vet -p 1 ./...`, `go build ./...`, and
  `git diff --check`: passed.

## Handoff

The only next operational step is a separately authorized CMS rehost. It must
not claim PostgreSQL integration or production verification before those are
actually performed.

## Completion

Archived after source verification. Publication is authorized separately.
