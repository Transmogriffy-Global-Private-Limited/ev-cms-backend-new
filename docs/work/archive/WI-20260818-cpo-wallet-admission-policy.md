# WI-20260818-cpo-wallet-admission-policy

Status: Completed
Owner: Anubhab Dey
Collaborators: Codex (implementation)
Started: 2026-08-18
Last updated: 2026-08-18

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — CMS HAL charging lifecycle

Detailed-plan reference: None

Issue/PR reference: None

## Outcome

Make CPO wallet minimum and buffer settings authoritative at each new charging
start, and guarantee a blank zero-default settings record for every CPO.

## Scope

- CPO settings API projection/update validation for wallet policy fields
- CPO provisioning and existing-CPO settings backfill
- Locked customer charging-start admission, hold, and HAL Wh-limit calculation
- OpenAPI, CPO frontend handoff, tests, and project memory

## Non-goals

- Changing settlement, wallet ledger behavior, tariff/GST selection, HAL
  protocol, customer time limits, or a live deployment

## Claimed surfaces

- `db/migrations/000043_*`, `src/models/settings.go`, `src/cpo/`, and
  `src/customerauth/charging.go`
- CPO settings/OpenAPI/frontend documentation

## Dependencies and blockers

- Existing migration 41 has non-negative integer, zero-default settings fields.
- Disposable PostgreSQL lifecycle checks require `TEST_DATABASE_URL`.

## Contract impact

- CPO settings expose and accept `wallet_min_balance` and
  `wallet_buffer_min_balance` whole-currency values.
- New starts require wallet balance at least the minimum and calculate
  affordability from `balance - buffer`.

## Current state

- Migration 43 backfills only missing CPO settings rows, and CPO provisioning
  creates the same blank row in its transaction.
- The CPO settings projection and multipart update accept non-negative,
  whole-currency `wallet_min_balance` and `wallet_buffer_min_balance` values.
- Every new charging start locks the CPO settings with the CPO and wallet,
  applies the minimum/buffer policy, then persists the resulting hold and HAL
  energy limit. Existing start replay, duration-cutoff, settlement, tariff/GST,
  and HAL protocol ownership are unchanged.

## Verification

- `go test ./src/cpo ./src/customerauth ./db -count=1` passed.
- `./scripts/verify-docs.ps1` passed.
- `go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1` passed.
- `go test -p 1 ./...`, `go vet ./...`, and `git diff --check` passed.
- PostgreSQL-backed provisioning and start-admission tests remain guarded by
  `TEST_DATABASE_URL`; they were not executed without an explicitly selected
  disposable database.

## Handoff

- Evaluate the settings and wallet inside the existing locked start transaction
  immediately before tariff/GST affordability and intent/hold creation.

## Completion

Completed 2026-08-18. Safe for migration-controlled deployment; no migration
or runtime operation was performed during this slice.
