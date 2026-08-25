# ADR 0014: Subscription Expiry Customer-Command Admission

Status: Accepted

Date: 2026-08-25

## Context

The CMS now records subscription expiry through the observed lifecycle worker,
but an elapsed period previously had no immediate effect on customer-paid User
App commands. The existing renewal endpoint only selected current records, so
it could not reactivate an `EXPIRED` subscription. Blocking every CPO or every
customer operation would strand active charging, payment settlement, and HAL
recovery, and would blur commercial policy with HAL-owned transaction truth.

## Decision

- Keep `cpos.status` as the independent authority for CPO administrative
  access; subscription status never activates or suspends a CPO administrator.
- Treat `EXPIRED`, and a current subscription whose period end has elapsed, as
  a gate only for creating a new customer charging start or a new customer
  wallet recharge order.
- Keep stop, existing start-intent replay, HAL fact delivery/reconciliation,
  settlement, customer reads, and verification of an existing recharge order
  available after expiry.
- Let `POST /api/v1/platform/cpos/{cpo_id}/subscription/renew` select and
  reactivate the expired record. It retains the existing audit, event, and
  idempotency evidence. An expired renewal cannot create another already
  elapsed period through a backdated start.
- Preserve legacy behaviour for CPOs with no subscription row. Absence is not
  reinterpreted as expiry by this narrowly scoped decision.

## Consequences

- The User App receives an explicit `403 cpo_subscription_expired` before a
  new commercial command creates a wallet hold, provider order, or HAL start.
- A SuperAdmin records manual payment confirmation through the existing renewal
  route rather than creating an out-of-band data repair or a second API.
- Platform invoices, platform payments, checkout, provider webhooks, general
  feature entitlements, and CPO administrative authorization remain outside
  this decision.
