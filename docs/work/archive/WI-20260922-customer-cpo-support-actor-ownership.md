# Customer-to-CPO support customer-actor ownership hardening

Status: complete (source-verified; PostgreSQL runtime gate skipped)

## Objective

Close the published migration 72 database-invariant gap: a CUSTOMER message
or event actor must be the exact customer that owns its CUSTOMER_CPO ticket.

## Claimed surfaces

- additive migration 73 and its deliberate rollback
- PostgreSQL-gated support integration coverage and migration source checks
- customer-support schema/project/changelog records

## Invariants

- CUSTOMER message `(ticket_id, author_customer_id)` and event
  `(ticket_id, actor_customer_id)` reference the ticket's exact `customer_id`.
- CPO_PLATFORM and CPO/PLATFORM actor rows keep their existing semantics.
- Existing valid history is preserved; migration preflight rejects inconsistent
  existing CUSTOMER actors rather than rewriting or deleting support history.

## Verification target

Focused migration/support tests, route parity regression, documentation
verification, full source checks, and strictly disposable PostgreSQL coverage
only when `TEST_DATABASE_URL` is explicitly supplied.

## Current source state

- Migration 73 uses ticket/customer composite foreign keys after an explicit
  mismatch preflight; migration 72 remains immutable.
- PostgreSQL integration coverage now exercises same-CPO and cross-CPO wrong
  customer actors, missing/mixed identities, valid owner actors, and a
  migration-73 rollback/reapply cycle.
- Passed: `gofmt`; focused DB/support and route/OpenAPI parity tests; docs
  verification; `go test -p 1 ./...`; `go vet -p 1 ./...`; `go build -p 1
  ./...`; and `git diff --check`.
- PostgreSQL-gated coverage was invoked with no `TEST_DATABASE_URL` and skipped
  with the exact reason `TEST_DATABASE_URL is not set`; no development/live
  database was selected. Migration 73 has not been deployed or applied to a
  runtime database.
- Source was committed and published to `main` and `anubhab-work`; no deploy,
  rehost/restart, SMTP contact, or HAL change occurred. This completed source
  item is archived.
