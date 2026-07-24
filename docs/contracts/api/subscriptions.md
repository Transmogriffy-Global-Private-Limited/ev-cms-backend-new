# Subscription and Entitlement API Contract

## Purpose and Boundary

This is the canonical human-readable contract for the platform subscription
surface. The machine-readable source is `docs/contracts/openapi/openapi.yaml`.

Every route requires an active `PLATFORM` bearer session. CPO and customer
sessions receive `403 forbidden`. No route exposes tenant business data,
Razorpay credentials, mail payloads, or authentication secrets.

A CPO may exist, authenticate, recover its accounts, and complete/reconcile
already-started operations without a subscription. Subscription and CPO
lifecycle are independent; expiry does not delete or silently suspend a CPO.

## Shared Rules

- Request bodies reject unknown fields and are limited to 32 KiB.
- Currency is a three-letter uppercase code.
- Money is an exact non-negative `price_minor` integer in the currency minor
  unit.
- All timestamps are RFC3339 UTC instants.
- Mutation reasons are required, trimmed, and at most 500 characters.
- Lifecycle `idempotency_key` values are required and at most 120 characters.
  Repeating the same key by the same actor for the same CPO and operation
  returns the original subscription. Reusing it for a different operation
  returns `409 idempotency_conflict`.
- Every successful mutation commits state, immutable audit, durable platform
  event, lifecycle history where applicable, and enabled CPO-admin mail work in
  one PostgreSQL transaction.
- Published versions and their entitlements are immutable at the database
  level. Editing a published plan creates a new draft version.
- Archived plans cannot publish or receive new assignments. Existing
  subscriptions and already-scheduled changes retain their version snapshot.

Common errors:

- `400 invalid_request` or `invalid_<field>`;
- `401 unauthorized`;
- `403 forbidden`;
- `404 <resource>_not_found`;
- `409 plan_conflict`, `subscription_conflict`,
  `invalid_subscription_transition`, or `idempotency_conflict`;
- `500 internal_error`.

## Data Shapes

Plan terms:

```json
{
  "currency": "INR",
  "price_minor": 250000,
  "billing_interval": "MONTHLY",
  "interval_count": 1,
  "trial_days": 14,
  "entitlements": [
    {
      "feature_key": "chargers.manage",
      "enabled": true,
      "limit_value": 100,
      "configuration": {}
    }
  ]
}
```

Feature keys are lowercase identifiers up to 120 characters, with `.`, `_`, or
`-` separators. Limits are optional non-negative integers. Configuration must
be a JSON object and contains non-secret feature policy only.

Subscription states:

```text
NONE -> TRIAL or ACTIVE
TRIAL -> ACTIVE, PAUSED, PAST_DUE, CANCELLED, EXPIRED
ACTIVE -> PAUSED, PAST_DUE, CANCELLED, EXPIRED
PAUSED -> ACTIVE, PAST_DUE, CANCELLED, EXPIRED
PAST_DUE -> ACTIVE, PAUSED, CANCELLED, EXPIRED
```

`CANCELLED` and `EXPIRED` are terminal. A CPO may later receive a new
subscription because the partial unique constraint covers only non-terminal
states.

## Plan Catalog

### `POST /api/v1/platform/plans`

Creates the plan and draft version 1.

Request:

```json
{
  "code": "growth_monthly",
  "name": "Growth Monthly",
  "description": "For expanding CPO networks.",
  "terms": {
    "currency": "INR",
    "price_minor": 250000,
    "billing_interval": "MONTHLY",
    "interval_count": 1,
    "trial_days": 14,
    "entitlements": [
      {
        "feature_key": "chargers.manage",
        "enabled": true,
        "limit_value": 100,
        "configuration": {}
      }
    ]
  }
}
```

Validation:

- `code`: unique lowercase underscore-separated identifier, maximum 80;
- `name`: required, maximum 150;
- `description`: maximum 2000;
- interval: `MONTHLY` or `YEARLY`;
- interval count: 1 through 120;
- trial days: zero through 365;
- entitlement keys must be unique inside the request.

`201 Created` returns `PlanView`: `plan`, optional `draft`, an array of
`published_versions`, and an `entitlements` object keyed by version UUID.

Side effects: `SUBSCRIPTION_PLAN_CREATED` audit and
`platform.subscription.plan_created`.

### `GET /api/v1/platform/plans`

Returns `{"plans":[PlanView...]}` newest plan first. This catalog is currently
unpaginated and intended for a bounded commercial catalog, not tenant data.

### `GET /api/v1/platform/plans/{plan_id}`

Returns one `PlanView`.

Additional errors: `400 invalid_plan_id`, `404 subscription_plan_not_found`.

### `PUT /api/v1/platform/plans/{plan_id}/draft`

Replaces plan name, description, draft commercial terms, and the complete draft
entitlement set.

Request is the create body without `code`:

```json
{
  "name": "Growth Monthly",
  "description": "Updated draft description.",
  "terms": {
    "currency": "INR",
    "price_minor": 275000,
    "billing_interval": "MONTHLY",
    "interval_count": 1,
    "trial_days": 7,
    "entitlements": []
  }
}
```

If a draft exists, it is replaced. If all versions are published, the server
creates `max(version)+1` as a new draft. An archived plan returns
`409 subscription_plan_archived`.

Side effects: `SUBSCRIPTION_PLAN_DRAFT_UPDATED` audit and
`platform.subscription.plan_draft_updated`.

### `POST /api/v1/platform/plans/{plan_id}/publish`

No body. Atomically changes the draft to `PUBLISHED`, records publisher/time,
and makes the plan `PUBLISHED`. The version and its entitlement rows then
reject updates and deletes.

Additional errors: `404 subscription_plan_draft_not_found`,
`409 subscription_plan_archived`.

Side effects: `SUBSCRIPTION_PLAN_PUBLISHED` and
`platform.subscription.plan_published`.

### `POST /api/v1/platform/plans/{plan_id}/archive`

No body. Idempotently changes the plan to `ARCHIVED`. It does not alter
published snapshots or current subscriptions.

Side effects on the first transition: `SUBSCRIPTION_PLAN_ARCHIVED` and
`platform.subscription.plan_archived`.

## CPO Subscription

### `POST /api/v1/platform/cpos/{cpo_id}/subscription`

Assigns a published, non-archived plan version only when the CPO has no
non-terminal subscription.

Request:

```json
{
  "plan_version_id": "9d875dfc-7ca6-4f91-8408-92ce4fa155a8",
  "starts_at": "2026-07-24T12:00:00Z",
  "reason": "Commercial agreement TGPL-2026-0042",
  "idempotency_key": "contract-tgpl-2026-0042"
}
```

`starts_at` is optional and cannot be future; server time is the default.
Trial days produce `TRIAL` plus `trial_ends_at`; otherwise status is `ACTIVE`.
The first period boundary is calendar-based with the published interval.

`201 Created` returns:

```json
{
  "subscription": {
    "id": "8e9a038d-545e-4644-a1ea-b5aa798290b8",
    "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
    "plan_version_id": "9d875dfc-7ca6-4f91-8408-92ce4fa155a8",
    "status": "ACTIVE",
    "starts_at": "2026-07-24T12:00:00Z",
    "current_period_starts_at": "2026-07-24T12:00:00Z",
    "current_period_ends_at": "2026-08-24T12:00:00Z",
    "cancel_at_period_end": false,
    "created_by": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
    "created_at": "2026-07-24T12:00:00Z",
    "updated_at": "2026-07-24T12:00:00Z"
  },
  "plan": {},
  "plan_version": {}
}
```

The `plan` and `plan_version` fields contain the full schemas in OpenAPI.

Side effects: initial lifecycle history, `CPO_SUBSCRIPTION_ASSIGNED`,
`platform.subscription.assigned`, and `CPO_SUBSCRIPTION_CHANGED` mail to active
CPO OWNER/ADMIN identities when mail is enabled.

### `GET /api/v1/platform/cpos/{cpo_id}/subscription`

Returns the current `TRIAL`, `ACTIVE`, `PAUSED`, or `PAST_DUE` subscription and
snapshot. A CPO legitimately may return `404 subscription_not_found`.

### `POST /api/v1/platform/cpos/{cpo_id}/subscription/change-plan`

Request:

```json
{
  "plan_version_id": "6e184bd5-4a02-46d6-94dc-9a9b7f8e87a5",
  "effective": "PERIOD_END",
  "reason": "Approved upgrade",
  "idempotency_key": "upgrade-2026-08"
}
```

- `IMMEDIATE` resets the period start to server time and calculates a new
  boundary using the target snapshot.
- `PERIOD_END` records `pending_plan_version_id` and `pending_change_at`; the
  lifecycle worker applies it transactionally at the boundary.

Scheduling and application each produce immutable history, audit, event, and
mail records. An already-scheduled version remains valid if its plan is later
archived.

### `POST .../subscription/pause`

Body:

```json
{
  "reason": "Commercial hold requested by CPO",
  "idempotency_key": "hold-2026-08"
}
```

Moves `TRIAL`, `ACTIVE`, or `PAST_DUE` to `PAUSED`.

### `POST .../subscription/resume`

Same body shape. Moves `PAUSED` or `PAST_DUE` to `ACTIVE`.

### `POST .../subscription/mark-past-due`

Same body shape. Explicitly moves `TRIAL`, `ACTIVE`, or `PAUSED` to
`PAST_DUE`. No provider webhook is implied.

### `POST .../subscription/expire`

Same body shape. Immediately makes the current subscription terminal
`EXPIRED`, sets `ended_at`, and clears scheduled changes/cancellation.

### `POST .../subscription/cancel`

Request:

```json
{
  "reason": "Agreement ended",
  "idempotency_key": "cancel-contract-0042",
  "at_period_end": true
}
```

- false/missing: immediately sets terminal `CANCELLED`, `cancelled_at`, and
  `ended_at`;
- true: retains current state and sets `cancel_at_period_end`; the lifecycle
  worker applies terminal cancellation at the recorded boundary.

### `GET .../subscription/history`

Returns `{"history":[...]}` newest first, limited to 500 records. Every record
contains prior/next state and version, actor, reason, idempotency key, effective
time, operation metadata, and creation time. History is never rewritten.

## Effective Entitlements and Overrides

### `GET /api/v1/platform/cpos/{cpo_id}/entitlements`

Resolution:

```text
current immutable plan-version entitlements
-> replace matching keys with non-expired CPO overrides
-> sort by feature key
```

A CPO without a subscription starts from the safe empty baseline and may still
have explicit overrides. Each resolved row has `source: PLAN|OVERRIDE`.
Expired overrides remain stored for audit but are ignored.

### `PUT /api/v1/platform/cpos/{cpo_id}/entitlement-overrides/{feature_key}`

Creates or replaces one override.

```json
{
  "enabled": true,
  "limit_value": 150,
  "configuration": {},
  "reason": "Temporary contracted expansion",
  "expires_at": "2026-12-31T18:30:00Z"
}
```

`expires_at` is optional and must be future. A reason is mandatory. The route
returns the stored override and emits
`platform.subscription.entitlement_override_set`.

### `DELETE /api/v1/platform/cpos/{cpo_id}/entitlement-overrides/{feature_key}`

Requires a URL-encoded `reason` query parameter:

```text
DELETE .../chargers.manage?reason=Temporary%20expansion%20ended
```

Returns `204 No Content`; missing override returns
`404 entitlement_override_not_found`. Audit stores the reason and event
`platform.subscription.entitlement_override_removed` invalidates clients.

## Worker, Failure, and Recovery

`subscription-lifecycle` sends an immediate and periodic durable heartbeat. It
uses row locking with `SKIP LOCKED`, so multiple process instances do not apply
one due transition concurrently. Each transaction processes one due row and
continues until no due work remains.

Processing priority when multiple boundaries coincide:

1. scheduled cancellation;
2. scheduled plan change;
3. trial completion.

If the process crashes before commit, no partial state/history/event/mail is
visible and the row remains due. If it crashes after commit, the durable result
exists; mail and realtime are independently retryable/at-least-once. The next
worker run catches up overdue boundaries. A stale registered lifecycle worker
degrades readiness through the shared worker-health contract.

## Verification

Focused:

```powershell
$env:GOCACHE = (Join-Path (Get-Location) '.local/go-cache')
go test ./src/subscriptions ./src/routes ./src/mail ./src/models ./db -count=1
```

PostgreSQL lifecycle:

```powershell
$env:TEST_DATABASE_URL = 'postgres://postgres@127.0.0.1:5432/ev_cms_test?sslmode=disable'
go test ./src/subscriptions -run TestSubscriptionLifecycleWithPostgreSQL -count=1
```

`TEST_DATABASE_URL` must identify a disposable database.
