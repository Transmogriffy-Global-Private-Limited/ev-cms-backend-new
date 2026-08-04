# ADR 0012: Manual Platform Subscriptions Without a Provider

Status: Accepted

Date: 2026-08-04

## Context

ADR 0008 correctly retired an unused prototype when product direction was
manual CPO activation only. Product direction now explicitly requires the
subscription catalog again, but no automatic
subscription provider, checkout, collection, payment webhook, invoice, or
background lifecycle processor exists.

## Decision

- Migration twelve restored subscription and entitlement tables; forward
  migration thirteen re-retires the two dormant feature-key tables. The four
  subscription catalog/history tables remain active, and platform billing
  remains retired.
- Platform superadmins manually create/publish/archive plans, issue and renew
  CPO subscriptions, switch plans, and choose every status transition.
- The application generates UUIDs, plan version numbers, timestamps, audit
  records, platform events, and idempotency evidence. Clients never supply
  those identifiers.
- Period and trial dates are recorded from plan terms, but crossing a date
  boundary never changes state. Activation, renewal, past-due, pause, resume,
  cancellation, and expiry are all explicit platform commands.
- No subscription command activates/suspends a CPO or otherwise substitutes
  for the CPO lifecycle authority in `cpos.status`.
- Feature keys and entitlement overrides are not exposed until a future
  product decision defines concrete modules and their server-side enforcement.
- `subscription-lifecycle` and `billing-maintenance` stay disabled and
  non-required. Platform billing remains retired in `retired_commercial`.
- No subscription email is produced in this slice.

## Consequences

- The CPO subscription lifecycle is auditable, idempotent, and controllable
  without pretending a provider exists.
- Operators must renew or transition subscriptions deliberately; expired dates
  are visible records, not automatic policy triggers.
- A future payment-provider integration requires a separate decision covering
  payment truth, reconciliation, webhook verification, retries, and how it
  interacts with this manual authority.
