# WI-20260910-charging-session-stop-provenance

Status: Verified
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-09-10
Last updated: 2026-09-10

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — charging lifecycle
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Persist distinct authoritative requested-stop initiator/reason and charger OCPP
stop reason in CMS, with matching immutable-fact and exact-reconciliation
projection and customer-readable session provenance.

## Scope

- CMS migration `000068`, session model, HAL transaction DTO/evidence, locked
  completion finalizer, customer detail/history views, OpenAPI, and tests.

## Non-goals

- HAL changes, migration application, deployment, economics, connector state,
  stop-command recovery, or frontend presentation copy.

## Claimed surfaces

- `db/migrations/000068_*`, `src/models/`, `src/halclient/`, `src/halops/`,
  `src/customerauth/`, OpenAPI, User App and integration documentation.

## Dependencies and blockers

- HAL already emits all three fields on exact transaction reads and immutable
  `transaction.completed` facts. PostgreSQL integration requires a disposable
  `TEST_DATABASE_URL`.

## Contract impact

- Additive customer-session `stop` object with requested initiator/reason and
  OCPP reason. Existing `stop_reason` remains a compatibility field.

## Data and migration impact

- Three nullable columns; no historical backfill because legacy provenance is
  ambiguous. Migration is source-only until explicitly applied.

## Current state

- Immutable HAL fact ingress and exact HAL reconciliation now pass the same
  distinct requested-stop/OCPP-stop evidence into the locked finalizer.
  Duplicate or missing-compatible evidence is idempotent, NULL fields may be
  filled, and conflicting established metadata returns an explicit conflict.

## Verification

- Passed focused `db`, `customerauth`, `halclient`, and `halops` tests;
  documentation/OpenAPI verification; full `go test -p 1 ./...`, `go vet -p
  1 ./...`, `go build ./...`, and `git diff --check`.
- PostgreSQL-gated migration/lifecycle coverage was not run because
  `TEST_DATABASE_URL` is unset.

## Handoff

- Do not collapse requested business provenance into OCPP `Remote`/other
  charger reasons. Conflicting established metadata is fail-safe.

## Completion

- Source is verified and ready for CMS-only publication. Migration `000068` is
  not applied; no deployment or restart occurred.
