# WI-20260910-cpo-stop-provenance-projection

Status: Verified
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-09-10
Last updated: 2026-09-10

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — charging lifecycle
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Expose existing canonical charging-session stop provenance in tenant-scoped CPO
session list/detail and charger-transaction projections.

## Scope

- CPO projection types/helpers, focused tests, OpenAPI, CPO contract and
  frontend handoff documentation.

## Non-goals

- HAL, migration, customer/User App behaviour, CPO authorization/query/live
  telemetry, settlement, deployment, and inference from legacy reason fields.

## Claimed surfaces

- `src/cpo/`, CPO OpenAPI/docs, and this work item.

## Dependencies and blockers

- Canonical fields are already persisted by migration `000068`. PostgreSQL
  integration execution requires a disposable `TEST_DATABASE_URL`.

## Contract impact

- Additive CPO `stop` object with the same requested/OCPP semantics as the
  customer projection. Existing `stop_reason` and transaction `reason` remain
  compatibility fields.

## Data and migration impact

- None.

## Current state

- One CPO-local helper projects only the already-loaded canonical session
  columns for session list/detail and charger transactions. CPO live telemetry
  remains unchanged because it intentionally represents ongoing live state.

## Verification

- Passed focused CPO projections, OpenAPI runtime-route coverage, documentation
  verification, full `go test -p 1 ./...`, `go vet -p 1 ./...`, `go build
  ./...`, and `git diff --check`.
- PostgreSQL-gated execution was skipped because `TEST_DATABASE_URL` is unset.

## Handoff

- Do not infer requested provenance from OCPP `Remote` or legacy fields.

## Completion

- Source is verified and ready for CMS-only publication. No migration, HAL
  change, deployment, or restart occurred.
