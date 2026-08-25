# Manual Platform Subscription API

This is the semantic companion to the canonical [OpenAPI contract](../openapi/openapi.yaml).
All endpoints are platform-superadmin-only and require `Authorization: Bearer`.
The server generates UUIDs, version numbers, timestamps, audit records, and
platform events. Write commands require a client-generated `idempotency_key`
and a human-readable `reason`; retry the same command with the same key.

## Lifecycle Authority

`cpos.status` controls CPO administrative access independently. Subscription
dates and states do not activate, suspend, cancel, renew, or otherwise
authorize a CPO administrator. The lifecycle worker records an elapsed current
period as `EXPIRED`; a SuperAdmin can then renew that same record after manual
payment confirmation.

An expired subscription has one deliberately narrow User App effect: it blocks
new `POST /api/v1/app/charging-sessions` and new
`POST /api/v1/app/wallet/recharge/orders` commands for that CPO. The command
gate is time-based, so it begins at `current_period_ends_at` even before the
worker has written `EXPIRED`. It does not block CPO administration, customer
reads, a retry that only returns an existing start intent, customer remote
stop, HAL fact delivery/reconciliation, charging settlement, or verification
of a recharge order created before expiry. CPOs with no subscription record
retain the legacy behaviour; absence is not interpreted as expiry.

`billing_interval` and `interval_count` describe the period recorded by manual
issue, renew, and immediate plan-change commands. An elapsed
`current_period_ends_at` is processed by the observed lifecycle worker. A
superadmin uses `renew` with a reason and new idempotency key to reactivate an
`EXPIRED` record; an attempted backdated reactivation starts at the renewal
time instead. Scheduled changes and cancellation at period end are unsupported.

## Operations

| Resource | Operations |
| --- | --- |
| Plan catalog | `POST/GET /api/v1/platform/plans`, `GET /plans/{plan_id}`, `PUT /plans/{plan_id}/draft`, `POST /publish`, `POST /archive` |
| CPO subscription | `POST/GET /api/v1/platform/cpos/{cpo_id}/subscription`, then explicit `/renew`, `/change-plan`, `/activate`, `/pause`, `/resume`, `/mark-past-due`, `/expire`, `/cancel`, and `GET /history` |

Plans begin as drafts. Publishing makes a version immutable and issueable.
Archiving prevents future issue/change-plan selection but preserves historical
subscription reads. A current CPO subscription has status `TRIAL`, `ACTIVE`,
`PAUSED`, or `PAST_DUE`; terminal `CANCELLED` and `EXPIRED` records remain in
history. One current subscription is enforced per CPO by PostgreSQL.

Feature keys and entitlement overrides are intentionally absent. The current
whole-CPO administrative control remains the independent `cpos.status`
lifecycle. The narrow customer command gate above is not a general entitlement
framework; a future module catalog needs explicit server-side gates before
feature-level subscription terms can be introduced.

## Durable Side Effects and Recovery

Each successful write commits its subscription change, audit row, and
safe platform event in one PostgreSQL transaction. Retry of a subscription
command with the same actor/key returns its already-recorded result; reusing a
key for a different CPO or operation returns `409 idempotency_conflict`.

Refresh affected REST resources after platform realtime events. Current event
names and safe payloads are documented in
`docs/contracts/realtime/platform-events.md`.
