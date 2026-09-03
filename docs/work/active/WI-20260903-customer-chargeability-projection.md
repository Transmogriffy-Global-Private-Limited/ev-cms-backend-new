# WI-20260903-customer-chargeability-projection

Status: Implemented and published; not deployed
Owner: Codex
Collaborators: None
Started: 2026-09-03
Last updated: 2026-09-03

Development-plan reference: Phase 5/6 customer charging projection
Detailed-plan reference: `docs/plans/customer-app-experience.md`
Issue/PR reference: None

## Outcome

Expose one authoritative, customer-specific, side-effect-free `can_charge`
projection for the current normal/AUTO charging start across User App charger
reads and a bounded batch REST/SSE surface.

## Scope

- `src/customerauth` customer charger projections, batch evaluator, routes,
  and SSE replacement snapshots.
- `src/operationalrealtime` wake-up cursor helper only.
- User App, realtime, and OpenAPI contracts plus focused tests.

## Non-goals

- HAL, frontend, StartCharging mutation semantics, migration, runtime database,
  deployment, commit, or push changes.
- Reserving wallet funds, mapping synchronization, HAL calls, command creation,
  or reconciliation from a read.

## Claimed surfaces

- `src/customerauth/network.go`, `chargeability.go`, `router.go`, and tests.
- `src/operationalrealtime/service.go`.
- `docs/contracts/openapi/openapi.yaml`, User App/realtime documentation.

## Dependencies and blockers

- Existing durable CMS/HAL runtime projection, commercial admission, tariff,
  GST, wallet, hold, start-intent, and session tables.
- PostgreSQL integration coverage is conditional on an explicitly supplied
  disposable `TEST_DATABASE_URL`; none is configured.

## Contract impact

- Additive `can_charge` and `chargeability_reason` fields on full
  `CustomerCharger` and `CustomerConnector` projections.
- New protected batch REST and SSE routes under `/api/v1/app/operations/`.
- Existing availability fields/routes remain compatible and unchanged in name.

## Data and migration impact

No migration. The projection reads existing authoritative state only.

## Current state

- Shared batched read evaluator is implemented and is wired through customer
  list/detail/hub/favorite/full selected-charger projections.
- Batch REST/SSE endpoints and full-replacement fingerprinting are implemented.
- The projection preserves materialized `OFFLINE`, `UNKNOWN`, and `STALE` live
  evidence as distinct chargeability reasons; it does not alter the existing
  lower-level operational-state semantics.

## Verification

- Focused chargeability, SSE serialization, and OpenAPI route checks: PASS.
- Repository-wide `go test ./...`, `go vet ./...`, `go build ./...`, docs
  verification, and `git diff --check`: PASS.
- PostgreSQL integration: SKIPPED because `TEST_DATABASE_URL` is not set.

## Handoff

Do not change StartCharging into a read projection. Its locks, mapping
synchronization, command delivery, and post-read rechecks remain final
authority even when `can_charge=true` was observed earlier.

## Completion

Source checkpoint `01cd88d` is verified and published. No migration or runtime
deployment is part of this work item.
