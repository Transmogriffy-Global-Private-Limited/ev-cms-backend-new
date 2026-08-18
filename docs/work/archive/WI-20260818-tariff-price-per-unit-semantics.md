# WI-20260818-tariff-price-per-unit-semantics

Status: Completed
Owner: Codex
Collaborators: Abhranil Pal (CPO tariff overlap); Anubhab Dey (User App and charging overlap)
Started: 2026-08-18
Last updated: 2026-08-18

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — Customer access/tariffs and charging lifecycle

Detailed-plan reference:

- `docs/plans/cpo-admin-network-configuration.md`
- `docs/plans/customer-app-experience.md`

Issue/PR reference: None

## Outcome

Correct the misleading tariff price name and make new charging admission,
reservation, snapshots, customer price views, and settlement share one explicit
tariff interpretation.

## Scope

- Forward data-preserving tariff-column rename and GORM/API contract rename
- Supported tariff-semantic validation for CPO tariff writes
- User App price, charging-start reservation, immutable snapshot, and
  settlement interpretation for energy, time, and per-session tariffs
- Historical `price_per_kwh` snapshot compatibility
- Focused tests and affected documentation/project memory

## Non-goals

- HAL/OCPP protocol, charger liveness, realtime behavior, deployment, live DB
  mutation, or unrelated wallet/recharge behavior
- Changing tariff target precedence (`USERGROUP > CHARGER > HUB`) or moving GST
  back onto tariffs
- Idle-fee billing; it remains separate until a trustworthy idle-time boundary
  is implemented

## Claimed surfaces

- `db/migrations/000040_*`
- `src/models/`, `src/cpo/`, and `src/customerauth/` tariff and charging paths
- Tariff/charging OpenAPI, API contracts, User App handoff, and project memory

## Dependencies and blockers

- Overlaps the active CPO-network-pricing and User-App work records. The
  user-authorized semantic correction is narrow and preserves their ownership
  boundaries.
- Disposable PostgreSQL lifecycle verification remains unavailable because
  `TEST_DATABASE_URL` is not configured.

## Contract impact

- New tariff writes and all current tariff reads use `price_per_unit`, with
  explicit `fixed`, `energy`, and `watt/hour` semantics.
- Historical charging snapshots may retain their old `price_per_kwh` key only
  for explicit compatibility during settlement and session display.

## Data and migration impact

- The forward migration renames `tariffs.price_per_kwh` without dropping or
  recreating data, and renames its check constraint.
- It deliberately does not invent missing semantic metadata for legacy tariff
  rows.

## Current state

- Audit found that old admission and settlement both hard-coded a kWh
  conversion. The user clarified that `sessions` is a fixed per-session tariff
  and `time` bills actual charger-originated session duration; the existing HAL
  duration limit remains only a safety cutoff. Existing snapshots omit tariff
  semantics and require an explicit legacy reader.

## Verification

- `gofmt` ran on all changed Go files.
- Focused `go test ./db ./src/customerauth ./src/cpo ./src/models -count=1`
  and follow-up `go test ./src/customerauth ./src/cpo -count=1` passed.
- `scripts/verify-docs.ps1`, the runtime/OpenAPI/Swagger route test, serial
  `go test ./...`, serial `go vet ./...`, Caddy validation, migration 40
  execution, local/public health/readiness, and protected route boundaries
  passed.
- The guarded admission/materialization lifecycle tests remain unavailable
  because `TEST_DATABASE_URL` is unset.

## Handoff

- Preserve the old snapshot key only in narrowly named compatibility code and
  test fixtures. No live tariff or new snapshot should use it.
- The existing `max_duration_seconds` remains a HAL/customer safety cutoff.
  Do not make tariff `price_type: time` alter that behavior.

## Completion

Completed and deployed on 2026-08-18. Migration 40 is applied and revision
`040b9bb` is active on the development VPS after the follow-on migration-41
settings release.
