# WI-20260818-customer-wallet-policy-projection

Status: Completed
Owner: Anubhab Dey
Collaborators: Codex (implementation)
Started: 2026-08-18
Last updated: 2026-08-18

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — CMS HAL charging lifecycle and wallet billing

Detailed-plan reference: None

Issue/PR reference: None

## Outcome

Expose the current CPO wallet admission policy and customer-specific usable
balance/recharge shortfall on each customer wallet read.

## Scope

- Customer wallet and wallet-history projections
- CPO settings read/fallback behavior for those projections
- OpenAPI and User App frontend contract
- Focused verification and project memory

## Non-goals

- Changing wallet mutation, recharge payment rules, session settlement, or
  charging-start policy

## Claimed surfaces

- `src/customerauth/wallet.go`, wallet tests, OpenAPI, User App handoff, and
  project-memory documentation

## Dependencies and blockers

- Depends on the same source slice's migration 43/settings policy. No
  disposable PostgreSQL database is configured for guarded lifecycle checks.

## Contract impact

- `GET /api/v1/app/wallet` and `/wallet/transactions` will return usable
  balance, CPO minimum/buffer, and the threshold-only recharge shortfall.

## Current state

- Customer wallet and wallet-history reads load the current CPO settings at
  request time. They return the actual balance, CPO whole-currency minimum and
  buffer, a non-negative post-buffer usable balance, and the threshold-only
  recharge shortfall.
- A temporary absent settings row reads as the zero-default policy during a
  rolling migration deployment. The charging-start transaction remains the
  authoritative locked admission decision.

## Verification

- `go test ./src/customerauth -count=1` passed.
- `./scripts/verify-docs.ps1` and OpenAPI/runtime-route parity passed.
- `go test -p 1 ./...`, `go vet ./...`, and `git diff --check` passed.
- Guarded PostgreSQL lifecycle tests were not run because no disposable
  `TEST_DATABASE_URL` is configured.

## Handoff

- Use the current CPO settings at read time. Clamp usable balance to zero and
  compute recharge amount only when raw wallet balance is below the minimum.

## Completion

Completed 2026-08-18. No migration or runtime operation was performed.
