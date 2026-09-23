# Customer-to-CPO support owner immutability

Status: complete (source-verified; PostgreSQL runtime gate skipped)

## Objective

Prevent a direct CUSTOMER_CPO ticket owner change from cascading into immutable
historical CUSTOMER message/event actor identity.

## Claimed surfaces

- additive migration 74 and schema-only rollback
- customer-support PostgreSQL-gated integration coverage and migration source checks
- customer-support schema, project-state, plan, changelog, and work records

## Invariants

- Migration 73 continues to bind CUSTOMER message/event actors to the ticket's
  exact `customer_id`.
- Migration 74 makes those composite foreign keys `ON UPDATE RESTRICT ON
  DELETE CASCADE`.
- A ticket-owner update that would rewrite historical CUSTOMER actor identity
  fails; no history is rewritten or deleted.
- CPO_PLATFORM and CPO/PLATFORM support behavior is unchanged.

## Verification target

Focused migration/support tests, route parity regression, documentation
verification, full source checks, and strictly disposable PostgreSQL coverage
only when `TEST_DATABASE_URL` is explicitly supplied.

## Current source state

- Migration 74 replaces only the two migration-73 ticket/customer composite
  foreign keys. Its down migration restores the exact migration-73 `ON UPDATE
  CASCADE ON DELETE CASCADE` behavior without changing rows.
- Source coverage verifies each named FK's update/delete rules, wrong-customer
  rejection, failed same-CPO ticket-owner mutation with ticket/message/event
  identity preservation, normal customer reply after rejection, and rollback/
  reapplication behavior.
- Passed: `gofmt`; focused DB/support and route/OpenAPI parity tests; docs
  verification; `go test -p 1 ./...`; `go vet -p 1 ./...`; `go build -p 1
  ./...`; and `git diff --check`.
- PostgreSQL-gated coverage was invoked with no `TEST_DATABASE_URL` and skipped
  with the exact reason `TEST_DATABASE_URL is not set`; no development/live
  database was selected. Migration 74 has not been deployed or applied to a
  runtime database.
- Source was committed and published to `main` and `anubhab-work`; no deploy,
  rehost/restart, SMTP contact, or HAL change occurred. This completed source
  item is archived.
