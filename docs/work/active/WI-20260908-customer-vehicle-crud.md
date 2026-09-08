# WI-20260908-customer-vehicle-crud

Status: Implemented (source verified; PostgreSQL lifecycle verification pending)
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-09-08
Last updated: 2026-09-08

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — customer app experience
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Provide customer-owned vehicle create, list, read, partial-update, and hard-delete
operations without exposing or accepting tenant/customer ownership identifiers.

## Scope

- Protected `/api/v1/app/vehicles` CRUD, strict request shapes, owner-scoped
  search/filter/keyset list, transactional audit records, and contract coverage.
- Forward migration `000066` that makes vehicle/customer tenant consistency a
  PostgreSQL composite foreign-key invariant.

## Non-goals

- CPO vehicle mutation, charging/session linkage, HAL changes, vehicle-number
  uniqueness or format rules, search infrastructure, runtime migration, or deploy.

## Claimed surfaces

- `src/customerauth`, vehicle model mapping, `000066` migrations, route/OpenAPI
  contract tests, HTTP contract, and this work record.

## Dependencies and blockers

- `000064` owns the vehicles table; no existing durable charging/session/invoice
  vehicle reference was found in the bounded implementation search.
- Database lifecycle tests require an explicitly selected disposable
  `TEST_DATABASE_URL` and remain unavailable locally.

## Contract impact

Adds five bearer-plus-app-ID customer routes. Foreign and absent vehicles share
the same `404 vehicle_not_found` response; ownership remains authentication-derived.

## Data and migration impact

`000066` replaces the standalone vehicle customer FK with
`(cpo_id, customer_id) -> customers(cpo_id, id)`. It does not repair bad rows,
add vehicle uniqueness, or alter charging data.

## Current state

All five customer routes, strict server-owned-field rejection, owner-scoped
search/filter/keyset reads, transactional audits, and hard delete are
implemented. The model now preserves nullable metadata and declares the
composite customer association. No migration has been run and no runtime state
has been changed.

## Verification

Focused customer-auth/routes/database tests, full `go test -p 1 ./...`,
`go vet -p 1 ./...`, OpenAPI/docs verification, route-contract smoke test, and
`git diff --check` pass. PostgreSQL lifecycle coverage is present in
`vehicles_integration_test.go` but remains skipped because `TEST_DATABASE_URL`
is unset.

## Handoff

Run the PostgreSQL-gated CRUD/atomicity test against an explicitly selected
disposable database before deployment, then archive this record.

## Completion

Source implementation complete; dynamic PostgreSQL verification pending.
