# Manual Platform Subscription API

This is the semantic companion to the canonical [OpenAPI contract](../openapi/openapi.yaml).
All endpoints are platform-superadmin-only and require `Authorization: Bearer`.
The server generates UUIDs, version numbers, timestamps, audit records, and
platform events. Write commands require a client-generated `idempotency_key`
and a human-readable `reason`; retry the same command with the same key.

## Lifecycle Authority

`cpos.status` controls CPO access independently. Subscription dates and states
do not automatically activate, suspend, expire, cancel, renew, or otherwise
authorize a CPO. No provider, checkout, invoice, payment, webhook, mail, or
worker participates in this API.

`billing_interval` and `interval_count` describe the period recorded by manual
issue, renew, and immediate plan-change commands. Passing `trial_ends_at` or
`current_period_ends_at` does nothing. A superadmin must invoke `activate`,
`renew`, `pause`, `resume`, `mark-past-due`, `expire`, or `cancel` explicitly.
Scheduled changes and cancellation at period end are unsupported.

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
whole-CPO service control remains the independent `cpos.status` lifecycle. A
future module catalog needs explicit server-side gates before feature-level
subscription terms can be introduced.

## Durable Side Effects and Recovery

Each successful write commits its subscription change, audit row, and
safe platform event in one PostgreSQL transaction. Retry of a subscription
command with the same actor/key returns its already-recorded result; reusing a
key for a different CPO or operation returns `409 idempotency_conflict`.

Refresh affected REST resources after platform realtime events. Current event
names and safe payloads are documented in
`docs/contracts/realtime/platform-events.md`.
