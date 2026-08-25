# WI-20260825-subscription-expiry-admission

Status: Completed in source
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-25
Last updated: 2026-08-25

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — Manual platform subscriptions
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

SuperAdmins can renew an expired CPO subscription, and expiration now blocks
only new customer charging starts and new customer wallet recharge orders.

## Scope

- The existing audited/idempotent renewal command reactivates an `EXPIRED`
  record and prevents a backdated renewal from creating another elapsed period.
- The expiry gate applies before new User App start/recharge-order side effects.
- Active-session stop/reconciliation and settlement, existing start replay, and
  pre-expiry recharge verification remain available.

## Non-goals

- CPO administrative access changes, payment-provider automation, subscription
  billing/invoicing, HAL/OCPP changes, or database deployment.

## Claimed surfaces

- `src/subscriptions`, `src/customerauth`, subscription/API contracts, tests,
  development/project memory, and ADRs.

## Dependencies and blockers

- The lifecycle worker and subscription migrations already exist in source.
- PostgreSQL-backed lifecycle/admission execution requires a disposable
  `TEST_DATABASE_URL` and was not run.

## Contract impact

- `POST /api/v1/platform/cpos/{cpo_id}/subscription/renew` reactivates an
  expired subscription after manual payment confirmation.
- New `POST /api/v1/app/charging-sessions` and
  `POST /api/v1/app/wallet/recharge/orders` calls return `403
  cpo_subscription_expired` when that CPO subscription has expired.

## Data and migration impact

- No schema change. Renewal updates the existing expired row and writes the
  existing audit/history/event evidence.

## Current state

- CPO administrative authority remains independent from subscription state.
- An absent subscription record retains legacy behaviour rather than being
  silently treated as expired.

## Verification

- Passed: focused customer-auth/subscription tests, `verify-docs.ps1`, OpenAPI
  route parity, `go test ./...`, `go vet ./...`, and `git diff --check`.
- Pending: PostgreSQL-backed lifecycle/admission execution is skipped without
  an explicitly disposable `TEST_DATABASE_URL`.

## Handoff

- Do not extend this narrow command gate to remote stop, HAL fact handling,
  reconciliation, settlement, customer reads, or verification of a recharge
  order created before expiry without a separate product decision.

## Completion

Completed in source; no database or deployment was changed.
