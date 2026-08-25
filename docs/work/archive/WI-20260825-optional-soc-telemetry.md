# WI-20260825-optional-soc-telemetry

Status: Completed (source verified; database and hardware acceptance unrun)
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-08-25
Last updated: 2026-08-25

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — Charging lifecycle and HAL integration
Detailed-plan reference: `docs/integrations/ocpp-hal-boundary.md`
Issue/PR reference: None

## Outcome

Carry charger-observed optional OCPP State of Charge (SoC) from HAL
MeterValues through durable immutable facts into CMS session, live, history,
and realtime projections without deriving or fabricating a value.

## Scope

- HAL/CMS telemetry fact contract, independent SoC ordering, durable session
  fields, migrations, customer projection, live freshness, realtime, tests,
  OpenAPI, and integration documentation.

## Non-goals

- Battery-capacity estimation, tariff/billing/limit behavior, a generic
  all-measurand telemetry platform, database deployment, or physical-charger
  operation.

## Claimed surfaces

- `src/customerauth`, `src/liveops`, `src/models`, migrations, OpenAPI,
  operational-event and HAL boundary documentation; the counterpart HAL work
  item claims OCPP parsing, durable transaction telemetry, and outbox facts.

## Dependencies and blockers

- Builds on the immutable `transaction.meter` fact path but uses an additive
  SoC fact to retain independent meter and SoC sequences.
- Disposable PostgreSQL and the CMS-to-HAL-to-cpconsole topology are required
  for full persistence and physical acceptance.

## Contract impact

- Adds optional charger-observed `transaction.soc` immutable facts and
  additive customer session/history fields. Missing SoC remains unknown;
  SoC freshness is independent from energy-meter freshness.

## Data and migration impact

- Adds an additive nullable `charging_sessions` SoC migration with 0–100
  constraints and no backfill.

## Current state

- CMS accepts the additive `transaction.soc` fact with independent ordering,
  persists nullable first/latest observed SoC, exposes it through customer,
  CPO, liveops, OpenAPI, history, and realtime projections, and never derives
  an absent value. Equal and older SoC timestamps cannot replace accepted truth.

## Verification

- Passed: focused `customerauth`, `liveops`, `models`, and `cpo` tests;
  `scripts/verify-docs.ps1`; route/OpenAPI/Swagger test; `go test -p 1 ./...`;
  `go vet -p 1 ./...`; and `git diff --check`.
- Not run: disposable PostgreSQL migration/projection tests and live
  CMS-to-HAL-to-cpconsole acceptance. No database or runtime was mutated.

## Handoff

- Preserve the separate HAL/CMS source-of-truth boundary. A SoC value must be
  actual charger evidence only; no missing value may become zero or an estimate.

## Completion

Source implementation complete. The separate HAL work item records the
counterpart parser, persistence, fact, and contract verification.
