# WI-20260818-tariff-patch-snapshot-hardening

Status: Completed
Owner: Anubhab Dey
Collaborators: Codex (implementation); coordinate with Abhranil Pal's active CPO pricing work
Started: 2026-08-18
Last updated: 2026-08-18

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — tariff/GST commercial correction and CMS HAL charging lifecycle

Detailed-plan reference: None

Issue/PR reference: None

## Outcome

Make tariff PATCH nullable-field intent explicit, validate the resulting tariff
state once across all target scopes, and validate frozen GST snapshot components
before immutable settlement.

## Scope

- `UpdateTariffRequest` JSON presence/null/value semantics
- Shared Hub/Charger/UserGroup tariff PATCH application and validation
- Frozen tax-snapshot component validation at settlement
- Focused tests, OpenAPI, CPO/User App handoffs, and project memory

## Non-goals

- Tariff targeting/precedence, GST Hub ownership, mutable settlement lookup,
  tariff numeric conversion, HAL/OCPP, wallet policy, migrations, deployment,
  or runtime database changes

## Claimed surfaces

- `src/cpo/` tariff request/service/tests
- `src/customerauth/` immutable tariff/tax calculation and tests
- `src/commercial/` reusable GST-component validation
- tariff contracts and frontend handoffs

## Dependencies and blockers

- Overlaps the active CPO pricing contract; keep the correction additive and
  scoped to PATCH and frozen-snapshot correctness.
- PostgreSQL lifecycle checks require an explicitly disposable
  `TEST_DATABASE_URL`.

## Contract impact

- PATCH distinguishes omitted, explicit `null`, and value for `units`,
  `start_date`, and `end_date`.

## Current state

- `NullablePatchField` now preserves omitted/null/value intent for tariff
  units and schedule fields.
- All three tariff PATCH scopes use `applyTariffUpdate`, then validate the
  complete in-memory result before writing it. `units:null` clears a sessions
  tariff and paired null dates clear a schedule.
- Frozen tax settlement now uses shared commercial component validation without
  reading mutable Hub/GST state. Historical tariff snapshot compatibility stays
  on its explicit released paths.

## Verification

- `go test ./src/cpo ./src/customerauth`
- `./scripts/verify-docs.ps1`
- `go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1`
- `$env:GOMAXPROCS='2'; go test -p 1 ./...; go vet -p 1 ./...`
- Disposable PostgreSQL lifecycle coverage remains pending an explicitly
  selected `TEST_DATABASE_URL`.

## Handoff

- Preserve legacy tariff snapshot compatibility and never use current Hub/GST
  state to settle an existing session.

## Completion

Completed and deployed on 2026-08-18 in runtime revision `a5d1af4`. No
migration or runtime database mutation was required; the guarded disposable
PostgreSQL lifecycle test remains pending `TEST_DATABASE_URL`.
