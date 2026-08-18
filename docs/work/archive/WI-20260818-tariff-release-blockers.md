# WI-20260818-tariff-release-blockers

Status: Completed
Owner: Anubhab Dey
Collaborators: Codex
Started: 2026-08-18
Last updated: 2026-08-18

## Outcome

Closed narrow application, database-boundary, and resolver-classification holes
in the temporal tariff fallback slice; the resulting migration is deployed.

## Claimed surfaces

- `src/cpo/`, `src/customerauth/`, `src/commercial/`, `db/migrations/`, and
  tariff/Hub contract documentation

## Non-goals

- tariff model redesign, GST/pricing changes, session-duration behavior, live
  database access, VPS access, and deployment

## Contract impact

- A Hub is always created hidden. Requesting `customer_visible:true` on
  creation returns `409 hub_tariff_root_required`; create a Hub root tariff
  first, then publish the Hub.
- Customer price routes and charging admission preserve infrastructure errors
  for normal 5xx handling while topology errors remain commercial absence.

## Data and migration impact

- Migration 44 gains a null-safe trigger that prevents tariff target identity
  mutation and is applied on the development database.

## Verification

- Passed database-free focused package tests, static migration checks, route
  and OpenAPI checks, documentation verification, `go test -p 1 ./...`, `go
  vet ./...`, and `git diff --check`.
- PostgreSQL disposable lifecycle/concurrency testing remains unperformed
  because no disposable `TEST_DATABASE_URL` was selected.

## Completion

- Hub creation validates the publication lifecycle before attempting a database
  write; trigger violations map to the same stable API error.
- Resolver callers distinguish topology failures from query/infrastructure
  failures.
