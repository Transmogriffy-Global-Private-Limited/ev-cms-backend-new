# Administrative HTTP API: Complete Developer Contract

## CPO staff authority and charging records

`GET /api/v1/cpo/customers` always includes `total_usage_kwh`, including zero.
`GET /api/v1/cpo/charging-sessions` and `/charger-transactions` identify the
CMS session separately from optional HAL/OCPP protocol identities, and include
the human-readable charger code, charger OCPP identity/name, hub name/address,
connector, payment/settlement, and reconciliation state needed for CPO support.

CPO staff use `/api/v1/cpo/access/me` as the current effective-permission
projection and use `/api/v1/cpo/permissions/catalog` plus `/api/v1/cpo/staff`
when their staff capabilities permit it. Roles are default bundles, not
endpoint authority.
An override has exactly one `ALLOW` or `DENY` effect; `DENY` wins over the role
default. Suspending or revoking a membership revokes its CPO sessions. Primary
administrator changes remain under the platform primary-admin flow so an ADMIN
cannot accidentally remove the tenant's recovery authority.

Support tickets are durable tenant conversations. CPO staff use
`/api/v1/cpo/support` and can only read or reply to their own CPO's tickets;
the platform uses `/api/v1/platform/support/tickets` for global support review,
replies, and status changes. The complete platform workflow, lifecycle,
privacy, retry, and unsupported-feature contract is
[`../../guides/workflows/superadmin-support-tickets.md`](../../guides/workflows/superadmin-support-tickets.md).
Mail layouts are source-controlled under
`src/mail/templates/`; the encrypted outbox remains the authoritative delivery
queue and message payload store.

## CMS HAL Operational Projections

Live operational REST snapshots are derived solely from committed CMS HAL
projections; these reads never block on a HAL HTTP request. `ONLINE`,
`OFFLINE`, and CMS administrative charger status are independent dimensions.
Connector `last_ocpp_status` is retained as protocol evidence, but a stale,
offline, or unknown parent connection makes customer/CPO live availability
`UNAVAILABLE` rather than `AVAILABLE`.

- `GET /api/v1/cpo/operations/fleet` requires active CPO membership, matching
  `X-CPO-App-ID`, and `chargers.operations`. It returns CPO-scoped projection aggregates only:
  charger connection counts, connector availability counts, and active-session
  count.
- `GET /api/v1/cpo/operations/chargers/{charger_id}` has the same capability authority
  and returns the existing administrative charger projection plus the current
  CMS live charger/connector projection. It does not grant command authority.
- `GET /api/v1/cpo/operations/live-sessions` is the primary live-table SSE.
  It immediately sends `event: snapshot`, then sends `event: live_sessions`
  after relevant committed CMS projection changes. Every `data` value is the
  complete current live-session response for the requested `limit` (default
  100, maximum 200), so the FE replaces its table state rather than applying
  meter/event patches. It contains only materialized `ACTIVE`, `STOP_PENDING`,
  and `RECONCILIATION_REQUIRED` sessions with display-safe charger/hub/connector
  context and committed meter/SoC freshness—never customer, wallet, tariff,
  amount, or settlement data. Each row additionally contains the CPO-visible
  `customer_name` and canonical CMS `connector_id`, plus `duration_seconds`
  measured from `started_at` to the payload's `as_of`. It never contains
  customer ID, email, or phone. Reconnect the stream for an immediate fresh
  snapshot; the server revalidates bearer, active membership, app ID, and fresh
  `chargers.operations` at heartbeat.
- `GET /api/v1/cpo/operations/live-sessions/snapshot` is the explicit JSON
  recovery/pagination route. It has the same authority and payload, and accepts
  the paired `after_started_at` and `after_id` cursor. It never calls HAL.
- `GET /api/v1/platform/cpos/{cpo_id}/operations/fleet` and
  `GET /api/v1/platform/cpos/{cpo_id}/operations/chargers/{charger_id}` require
  a `PLATFORM` bearer and are observation-only. They do not expose RemoteStop
  or other charger control.
- Every scope has cursor recovery and an SSE companion: CPO members with
  `chargers.operations` use
  `/api/v1/cpo/operations/events` and `/operations/realtime/stream`; Platform
  uses the corresponding route under the selected CPO; the User App uses
  `/api/v1/app/operations/events` and `/operations/realtime/stream` with its
  matching app-ID. All use `after_id` or `Last-Event-ID`, deliver IDs in order,
  may redeliver after reconnect, and revalidate the bearer session at each
  heartbeat. They contain only safe committed-change hints; callers must fetch
  the CPO fleet/charger or customer session projection after an event.
- `GET /api/v1/cpo/operations/live-sessions/events` remains the filtered
  durable `CHARGING_SESSION` cursor/reconciliation surface for advanced
  consumers. It is not needed to drive the normal live table. The old
  `/api/v1/cpo/operations/live-sessions/realtime/stream` is a deprecated alias
  of the primary full-snapshot SSE route.

The User App charging-session snapshot uses the same internal live capability,
but remains owner-only and never returns HAL bearer material, raw HAL IDs,
provider diagnostics, another customer session, or fabricated electrical
measurements. REST is the recovery authority; no frontend correctness depends
on a live connection.

## CPO typed charger operations

The CPO-only routes under `/api/v1/cpo/operations/chargers/{charger_id}` use
the CMS charger UUID (not the public inventory identifier), a matching active
`X-CPO-App-ID`, a CPO bearer, and `chargers.operations`. Every mutating route
also requires an `Idempotency-Key` of at most 128 characters. CMS persists the
operation before contacting HAL and uses the server request ID as the trusted
cross-service correlation ID. Reusing a key with an identical operation
returns the same durable record; reusing it with a different typed payload
returns `409 idempotency_conflict`.

- `POST /reset` accepts only `SOFT` or `HARD` plus a bounded reason.
- `POST /unlock` requires one CPO-owned CMS connector UUID.
- `POST /availability` accepts only `OPERATIVE` or `INOPERATIVE`, for the
  whole charger or one CPO-owned connector.
- `POST /clear-cache` has no generic payload.
- `POST /trigger-message` accepts only `BootNotification`,
  `DiagnosticsStatusNotification`, `FirmwareStatusNotification`, `Heartbeat`,
  `MeterValues`, or `StatusNotification`, optionally for one connector.
- `GET /configuration` reads a bounded repeatable `key` query parameter;
  values marked sensitive are explicitly redacted. `POST /configuration`
  additionally requires `chargers.manage`, rejects HAL-owned reconciliation
  keys and sensitive credential-like keys, and changes exactly one remaining
  key.
- `GET /api/v1/cpo/operations/charger-operations/{operation_id}` is the
  recovery resource. For `RECONCILIATION_REQUIRED` it performs only exact
  same-ID HAL lookup; it never sends another physical OCPP command.

`GET /api/v1/cpo/operations/charger-operations` is the separate CMS-owned
history feed. It requires the same CPO bearer, matching app ID, and
`chargers.operations` permission, but it is read-only: it neither contacts HAL
nor reconciles or changes any row. It uses deterministic keyset pagination:
`created_at DESC, id DESC`, default `limit=50`, maximum `200`; `before` and
`before_id` are a required RFC3339/UUID cursor pair. The response contains
`operations`, `has_more`, and the next cursor when another page exists.

The list accepts bounded `charger_id`, `connector_id`, `actor_user_id`, `kind`,
`state`, `ocpp_result`, `failure_category`, `created_after`, and
`created_before` filters. `reset_type`, `availability_type`,
`requested_message`, and `configuration_key` are typed parameter filters and
automatically select their matching operation kind; more than one typed
parameter filter or a mismatched explicit `kind` is rejected. It returns safe
charger code/name, optional connector number/type, and actor ID/name context.
Its `parameters` object is kind-specific: Reset includes type/reason,
availability includes type, TriggerMessage includes requested message, and
ChangeConfiguration includes only key. It never returns the configuration
value, raw JSONB, internal connector helper, idempotency key/digest,
correlation ID, or provider payload.

`PERSISTED` means CMS accepted the operation durably. `OCPP_CONFIRMED` means
HAL received an OCPP response, whose exact result is retained in
`ocpp_result`; it does not prove a later physical effect. `CONFIRMED_ABSENT`
means HAL's exact lookup proves the operation was never created there. A
`RECONCILIATION_REQUIRED` response is honest about ambiguous delivery. These
operations are distinct from customer RemoteStart/RemoteStop, sessions,
wallets, chargeability, firmware, and diagnostics; none of those states are
changed by this API.

This is the human-readable contract for every currently implemented HTTP
endpoint, including public customer signup and administrative APIs. It is
intended to be sufficient for frontend, QA, mobile, and backend integration
without reading Go source.

The machine-readable equivalent is `../openapi/openapi.yaml`. While the service
is running:

- interactive Swagger UI: `GET /docs/`
- raw OpenAPI 3.1: `GET /openapi.yaml`

The runtime/spec drift test fails when an implemented method/path is absent from
OpenAPI or OpenAPI advertises an operation the router does not implement.

## 1. Base Rules

### Origin and version

The default local origin is `http://127.0.0.1:8080`. Business endpoints use the
`/api/v1` prefix. Health and documentation routes are unversioned.

### Media type and JSON parsing

Requests with bodies use `Content-Type: application/json`. The server:

- accepts one JSON object;
- rejects malformed JSON;
- rejects unknown object fields;
- rejects a second JSON value after the object;
- limits bodies to 32 KiB.

These failures return:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "The request body is invalid."
  }
}
```

An oversized auth request uses message `The request body is too large.` with
the same code.

### Time, identifiers, and omitted fields

- IDs are UUID strings unless explicitly described as app IDs.
- Times are UTC RFC 3339 strings.
- Scope-specific optional fields are omitted, not returned as fabricated empty
  values.
- Empty collections are returned as `[]`.

### Request correlation

Every request that reaches Gin receives a new server-generated UUID in the
`X-Request-ID` response header. Browser clients may read it under the current
permissive CORS mode. Preserve it with failed-operation diagnostics so an
operator can find the matching structured completion record; do not attach
request bodies, credentials, or tokens. Client-supplied request IDs are not
adopted.

The backend request log records method, matched route template, status,
latency, response size, safe authenticated identifiers, and the stable handled
API error code. It deliberately excludes raw paths, queries, bodies, header
values, API messages, and secrets. The canonical log contract is
`../internal/http-request-logging.md`.

### Cache policy

Authentication, platform-CPO, and CPO-integration responses set:

```http
Cache-Control: no-store
Pragma: no-cache
```

Clients must still avoid logging or persisting OTPs, passwords, access tokens,
refresh tokens, or provider credentials.

### Error envelope

Every handled API failure uses:

```json
{
  "error": {
    "code": "stable_machine_code",
    "message": "Safe explanation for a person."
  }
}
```

`500 internal_error` deliberately omits database, cryptographic, SMTP, and
stack details.

## 2. Authentication and Tenant Headers

Protected routes require:

```http
Authorization: Bearer <access_token>
```

The access token is a signed-then-encrypted JWT. It is short-lived, but every
protected request also checks the durable session, user, scope authority,
membership, and CPO status in PostgreSQL.

CPO integration routes additionally require:

```http
X-CPO-App-ID: <current_app_id>
```

The bearer session establishes the CPO first. The app-ID header is then compared
with that CPO's current value. It cannot authenticate a caller, select another
CPO, or expand a role.

Swagger UI exposes both values through its **Authorize** dialog. Enter only the
token value for `BearerAuth`; Swagger UI adds `Bearer`. Enter the current app ID
for `CPOAppID`.

## 3. Health and Documentation

### `GET /health/live`

Authentication: none.

Purpose: process liveness only. No database query occurs.

`200 OK`:

```json
{"status":"ok"}
```

### `GET /health/ready`

Authentication: none.

Purpose: PostgreSQL readiness with a two-second query timeout.

`200 OK`:

```json
{"status":"ready"}
```

`503 Service Unavailable`:

```json
{"status":"not_ready"}
```

### `GET /openapi.yaml`

Authentication: none.

Returns the embedded canonical OpenAPI document as
`application/yaml; charset=utf-8`. The response uses `Cache-Control: no-cache`.

### `GET /docs` and `GET /docs/`

Authentication: none. `/docs` redirects temporarily to `/docs/`. The latter
serves embedded Swagger UI assets and loads `/openapi.yaml`. No public CDN is
required.

## 4. Customer/App-User Authentication

These endpoints are public and do not accept a bearer token. Every request
requires:

```http
X-CPO-App-ID: <current-dummy-or-live-app-id>
```

The header resolves the intended active CPO. It may be hardcoded into that
CPO's frontend because it is not a secret, but it does not authenticate a
person or protect against abuse. The server uses durable source/email rate
limits independently.

Signup has no commercial or payment check. Customer authentication uses a distinct
`CUSTOMER` session scope; customer tokens are not CPO-staff or platform tokens.

### 4.1 `POST /api/v1/app/auth/signup`

Request:

```json
{
  "email": "driver@example.com",
  "password": "<10-to-128-character-password>",
  "full_name": "Example Driver",
  "phone": "+919876543210"
}
```

Validation and normalization:

- email is trimmed, lowercased, syntactically valid, and at most 320
  characters;
- password is 10 to 128 characters;
- full name is trimmed and contains 1 to 255 characters;
- phone is optional; when supplied it is 7 to 15 digits with an optional
  leading `+`;
- unknown fields, multiple JSON objects, and bodies over 32 KiB are rejected;
- the app ID must currently belong to an `ACTIVE` CPO.

`202 Accepted`:

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
  "expires_at": "2026-07-23T12:10:00Z",
  "resend_available_at": "2026-07-23T12:01:00Z"
}
```

The transaction stores the proposed profile and an Argon2id password hash,
never the plaintext password, and queues an encrypted `CUSTOMER_SIGNUP_OTP`
mail job. It deliberately does not create a `customers` row yet. Consumed,
attempt-exhausted, and resend-invalidated challenges scrub their obsolete
password-hash copy.

Errors:

- `400 invalid_request`, `missing_cpo_app_id`, `invalid_email`,
  `invalid_password`, `invalid_full_name`, or `invalid_phone`;
- `403 signup_unavailable` for an unknown, stale, pending, or suspended CPO app
  identity;
- `429 rate_limited`;
- `503 mail_unavailable`;
- `500 internal_error`.

### 4.2 `POST /api/v1/app/auth/signup/verify`

Request:

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
  "code": "123456"
}
```

`201 Created`:

```json
{
  "customer_id": "e8a751ff-d7d4-4ce8-ab30-cdd8c8111363",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "wallet_id": "5bd431a7-63f0-4df7-a2f5-1b55112df560"
}
```

The server locks the challenge, revalidates the same active CPO/app ID, checks
the single-use OTP and attempt/expiry limits, and serializes work for the
normalized email. One transaction then:

1. creates the active, verified CPO-local `customers` account from the pending
   credentials and profile;
2. creates its zero-balance `INR` wallet;
3. consumes the challenge and scrubs its password-hash copy;
4. writes a tenant audit event.

Uniqueness is `(cpo_id, normalized email)`. The same email may create a wholly
independent account under another CPO with the same or a different password;
later profile, password, lockout, challenge, and session changes never cross
that boundary.

Errors:

- `400 invalid_request` or `missing_cpo_app_id`;
- `401 invalid_challenge` for bad, expired, consumed, invalidated, exhausted,
  or cross-CPO challenge/code;
- `403 signup_unavailable` if the CPO app identity is unavailable;
- `409 customer_already_registered` when the normalized email is already an
  account under this CPO;
- `429 rate_limited`;
- `500 internal_error`.

### 4.3 `POST /api/v1/app/auth/signup/resend`

Request:

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497"
}
```

After the reported cooldown, `202 Accepted` invalidates the prior challenge and
returns a replacement `ChallengeResponse`. The replacement keeps the same
pending profile and password hash, generates a new OTP, and revalidates the
active CPO/app ID. The old OTP is unusable.

Errors: `400 invalid_request` or `missing_cpo_app_id`,
`401 invalid_challenge`, `429 rate_limited`, `503 mail_unavailable`, or
`500 internal_error`.

### 4.4 `POST /api/v1/app/auth/login`

Request:

```json
{
  "email": "driver@example.com",
  "password": "<password>"
}
```

Requires the current `X-CPO-App-ID`. The server resolves that active CPO,
verifies the password, lockout state, and `ACTIVE` status of the customer
account found by `(CPO, normalized email)`. Invalid email, password, lockout,
status, and missing account share `401 invalid_credentials`.

`202 Accepted` returns `ChallengeResponse` and queues encrypted
`CUSTOMER_LOGIN_OTP` mail. No session exists yet.

Other errors: `400 invalid_request` or `missing_cpo_app_id`,
`403 signup_unavailable` for an unknown/inactive CPO app ID,
`429 rate_limited`, `503 mail_unavailable`, or `500 internal_error`.

### 4.5 `POST /api/v1/app/auth/login/verify`

Request:

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
  "code": "123456"
}
```

The challenge, active CPO-local customer account, active CPO, and
same current app ID are revalidated transactionally. Success consumes the OTP,
creates a durable customer session tied to `customer_id` and `cpo_id`, stores
one hashed refresh token, and returns:

```json
{
  "access_token": "<signed-and-encrypted-JWT>",
  "access_token_expires_at": "2026-07-23T12:15:00Z",
  "refresh_token": "<opaque-one-time-token>",
  "session_expires_at": "2026-08-22T12:00:00Z",
  "token_type": "Bearer",
  "customer_id": "e8a751ff-d7d4-4ce8-ab30-cdd8c8111363",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "cpo_app_id": "cpo_dummy_735f36a898b84ce68a350db38c90bf9b"
}
```

The JWT contains `CUSTOMER` scope and CPO context but not a staff role.
Customer authority remains the durable session/customer record.

Errors: `400 invalid_request` or `missing_cpo_app_id`,
`401 invalid_challenge`, `429 rate_limited`, or `500 internal_error`.

### 4.6 `POST /api/v1/app/auth/login/resend`

Body: `{"challenge_id":"<uuid>"}`.

After the cooldown, `202 Accepted` invalidates the old login challenge and
returns a replacement `ChallengeResponse`. It revalidates the CPO-local
customer, CPO, and app ID. The old OTP cannot be used.

Errors: `400 invalid_request` or `missing_cpo_app_id`,
`401 invalid_challenge`, `429 rate_limited`, `503 mail_unavailable`, or
`500 internal_error`.

### 4.7 `POST /api/v1/app/auth/refresh`

Request:

```json
{"refresh_token":"<current-opaque-refresh-token>"}
```

Requires the CPO app ID but no bearer token. Success returns the same customer
token response as login verification and atomically replaces the refresh
token. The client must discard the submitted token. Reuse of a consumed token
revokes that entire customer session.

Every refresh revalidates the active customer account, CPO,
customer-bound session, and current app ID.

Errors: `400 invalid_request` or `missing_cpo_app_id`,
`401 invalid_refresh_token`, `429 rate_limited`, or `500 internal_error`.

### 4.8 `GET /api/v1/app/me`

Requires bearer customer access token plus matching `X-CPO-App-ID`.

`200 OK`:

```json
{
  "user": {
    "id": "e8a751ff-d7d4-4ce8-ab30-cdd8c8111363",
    "email": "driver@example.com",
    "full_name": "Example Driver",
    "phone": "+919876543210",
    "is_verified": true,
    "last_login_at": "2026-07-23T12:00:00Z"
  },
  "customer": {
    "id": "e8a751ff-d7d4-4ce8-ab30-cdd8c8111363",
    "status": "ACTIVE",
    "user_group_id": "b82f047f-8ab6-4fbd-a7ba-b646f434eb01"
  },
  "cpo": {
    "id": "c821a013-5041-42f7-80c8-aa153cf9d455",
    "business_name": "Example Charging Private Limited",
    "app_id": "cpo_dummy_735f36a898b84ce68a350db38c90bf9b",
    "app_id_mode": "DUMMY"
  },
  "wallet": {
    "id": "5bd431a7-63f0-4df7-a2f5-1b55112df560",
    "balance": "0.00",
    "currency": "INR"
  }
}
```

Optional `phone`, `last_login_at`, and `user_group_id` are omitted when absent.
Money is an exact decimal string, not a JSON float.
The `user` key is retained for frontend compatibility, but it is a projection
of the same CPO-local account: `user.id` always equals `customer.id` and does
not identify a row in the administrative `users` table.

Errors: `400 missing_cpo_app_id`, `401 unauthorized`,
`403 cpo_app_id_mismatch`, or `500 internal_error`.

### 4.9 `PATCH /api/v1/app/profile`

Requires bearer customer access token plus matching `X-CPO-App-ID`.

Request:

```json
{
  "full_name": "Asha Das",
  "phone": "+919876543210"
}
```

`full_name` is required, trimmed, and limited to 255 characters. `phone` is
optional and must contain 7 to 15 digits with an optional leading `+`. Omit it
to preserve the current value; send explicit JSON `null` to clear it. Email is
not editable here and password changes remain owned by the password endpoints.

The customer and CPO are derived from the validated session. The request has
no customer, CPO, status, group, wallet, session, or identifier fields. The
transaction updates only this CPO-local customer account and, when a value
actually changes, writes `CUSTOMER_PROFILE_UPDATED` audit evidence containing
the changed field names and no old/new personal values.

`200 OK` returns the canonical `UserView` projection:

```json
{
  "id": "e8a751ff-d7d4-4ce8-ab30-cdd8c8111363",
  "email": "driver@example.com",
  "full_name": "Asha Das",
  "phone": "+919876543210",
  "is_verified": true,
  "last_login_at": "2026-07-23T12:00:00Z"
}
```

The authentication route group returns `Cache-Control: no-store`.
Errors: `400 invalid_request`, `400 invalid_full_name`, `400 invalid_phone`,
`400 missing_cpo_app_id`, `401 unauthorized`, `403 cpo_app_id_mismatch`, or
`500 internal_error`.

### 4.10 `GET /api/v1/app/hubs`

Requires a customer bearer token and matching `X-CPO-App-ID`. Returns only
customer-visible hubs belonging to the authenticated customer's CPO. `limit`
is 1–100 and defaults to 25; `before` and `before_id` are a paired descending
`(created_at, id)` keyset cursor. `q` searches the hub name or address. When
the filter changes, the frontend must discard the old cursor.

The response contains safe hub projections, `charger_count`, and the
customer-owned `is_favorite` flag. It never returns the CPO ID, sanctioned
load, audit data, or unpublished hubs. Errors include `400 invalid_limit`,
`400 invalid_cursor`, `400 invalid_search`, authentication/header errors, or
`500 internal_error`.

### 4.11 `GET /api/v1/app/hubs/{hub_id}`

Returns a published hub in the current CPO and its attached chargers and
connectors. Independent chargers are excluded. The projection includes only
public charger ID, vendor, model, maximum power, OCPP version, connector
capability, and customer favorite flags. It excludes OCPP identity, serial
number, sanctioned load, last-seen data, and audit information.

Each charger and connector also includes its DB-backed CMS administrative
`status` (`ACTIVE`, `INACTIVE`, `SUSPENDED`, `UNDERMAINTENANCE`, or
`DECOMMISSIONED`), and each connector includes
`connector_total_capacity`.
The former connector `max_current` and `max_voltage` fields are absent. The CMS
status is not live availability: until a separate CMS/HAL contract is
implemented, charger and connector `availability` is always `UNKNOWN`; this
endpoint makes no HAL call and does not claim live or online state. Unknown,
unpublished, and cross-CPO hubs all return `404 hub_not_found`.

### 4.12 `GET /api/v1/app/chargers/{charger_id}`

Uses the six-character public charger ID, not the CMS charger UUID. The charger
must belong to the authenticated CPO and be attached to a customer-visible hub;
otherwise the response is `404 charger_not_found` without cross-tenant
enumeration. The response uses the same safe projection and explicit
`availability: "UNKNOWN"` described in 4.11. Malformed IDs return
`400 invalid_charger_id`.

### 4.12A `GET /api/v1/app/chargers/{charger_id}/image`

Uses the same six-character public charger ID, customer bearer token,
`X-CPO-App-ID`, current-CPO, and published-hub rules as charger detail. It
serves only a regular JPEG, PNG, GIF, or WebP file from the existing charger
upload directory. The stored filesystem path is never returned or used as a
request input.

`CustomerCharger.charger_image_url` contains this relative API path only when
an uploaded image is recorded. Because the endpoint is authenticated, frontend
clients must request it with their normal auth headers and render the fetched
blob; a bare `<img src>` request cannot provide those headers. It returns
`404 charger_image_not_found` when the published charger has no safe image or
the stored file is absent.

### 4.13 Backend current-customer helpers

After `service.Authenticate()` and `customerauth.RequireAppID()` succeed,
backend app handlers use:

```go
principal, ok := customerauth.CurrentPrincipal(ctx)
customerID, ok := customerauth.CurrentCustomerID(ctx)
cpoID, ok := customerauth.CurrentCPOID(ctx)
appID, ok := customerauth.CurrentCPOAppID(ctx)
```

`Principal` already contains the trusted account, customer, CPO, wallet, and
session context used by the `me` response. These values come from the encrypted
token plus authoritative PostgreSQL revalidation. An app handler must not take
`customer_id` or `cpo_id` from its request body to establish ownership.
`CurrentUserID` remains as a source-compatibility alias and returns the exact
same value as `CurrentCustomerID`; new app code should use `CurrentCustomerID`.

### 4.14 `GET /api/v1/app/auth/sessions`

Requires customer bearer token and matching app ID. Returns only active,
unexpired `CUSTOMER` sessions for the current `customer_id` in the current CPO:

```json
{
  "sessions": [
    {
      "id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
      "ip_address": "127.0.0.1",
      "user_agent": "Example Mobile App",
      "created_at": "2026-07-23T12:00:00Z",
      "last_seen_at": "2026-07-23T12:05:00Z",
      "expires_at": "2026-08-22T12:00:00Z",
      "is_current": true
    }
  ]
}
```

It does not expose platform, CPO-staff, or another CPO's customer sessions.

### 4.15 `DELETE /api/v1/app/auth/sessions/{session_id}`

Requires customer bearer token and matching app ID. `204 No Content` revokes
the selected session and unused refresh token only when it belongs to this
customer/CPO. The current session may revoke itself.

Errors: `400 invalid_session_id` or `missing_cpo_app_id`,
`401 unauthorized`, `403 cpo_app_id_mismatch`, `404 session_not_found`, or
`500 internal_error`.

### 4.16 `POST /api/v1/app/auth/logout`

Requires customer bearer token and matching app ID.

`204 No Content` revokes the current session and unused refresh token.

### 4.17 `POST /api/v1/app/auth/logout-all`

Requires customer bearer token and matching app ID.

`204 No Content` revokes every customer session for this exact customer/CPO.
It deliberately does not revoke platform sessions, CPO-staff sessions, or
customer sessions for another CPO.

### 4.18 `POST /api/v1/app/auth/password/forgot`

Request: `{"email":"driver@example.com"}` plus the app-ID header.

For a valid active CPO, malformed, unknown, non-customer, and eligible emails
all return the same `202 Accepted` message:

```json
{
  "message": "If the customer account is eligible, a password reset code will be sent."
}
```

Only an active customer receives an encrypted
`CUSTOMER_PASSWORD_RESET_OTP` job. Account lockout does not prevent recovery.

That eligible recipient's encrypted email contains the opaque recovery ID
(`challenge_id`), six-digit code, and shared expiry. The generic response does
not expose whether the ID is real, so account enumeration remains blocked.

### 4.19 Customer password-reset resend and completion

Both operations use the recovery ID delivered in the eligible recipient's
email. Resend returns a replacement challenge response and mails a replacement
ID/code pair; the old pair is invalidated.

`POST /api/v1/app/auth/password/reset/resend`

```json
{"challenge_id":"<uuid>"}
```

Returns a replacement `ChallengeResponse` after cooldown.

`POST /api/v1/app/auth/password/reset`

```json
{
  "challenge_id": "<uuid>",
  "code": "123456",
  "new_password": "<10-to-128-character-password>"
}
```

Success returns `200 OK`, replaces this CPO-local customer password, clears its
lockout state, and revokes every customer session for this exact
`(cpo_id, customer_id)` account. Administrative and other-CPO accounts are
untouched. Any other unconsumed login/reset challenge for this account is also
invalidated so a pre-change OTP cannot create a post-change session.

### 4.20 `POST /api/v1/app/auth/password/change`

Requires customer bearer token and matching app ID.

```json
{
  "current_password": "<current-password>",
  "new_password": "<10-to-128-character-password>"
}
```

Success returns `200 OK` and revokes every session for this CPO-local customer,
including the current session, and invalidates every outstanding login/reset
challenge for that account. Errors include
`400 invalid_password`, `400 password_reused`,
`401 invalid_current_password`, normal bearer/app-ID errors, and
`500 internal_error`.

### 4.21 `GET /api/v1/app/favorites`

Returns the current customer's saved hubs and chargers as safe published
network projections. Hubs and chargers use independent bounded keyset cursors:
`hub_before` with `hub_before_id`, and `charger_before` with
`charger_before_id`. `limit` is 1–100 and defaults to 25. A favorite whose
resource was later unpublished is omitted; no unpublished inventory is leaked.

### 4.22 `PUT /api/v1/app/favorite-hubs/{hub_id}`

Requires a published same-CPO hub and creates the composite
`(cpo_id, customer_id, hub_id)` favorite idempotently. It returns `204 No
Content` whether the favorite was newly created or already present. The
transaction records a `CUSTOMER_FAVORITE_HUB_ADDED` audit action only for a
new row. Unknown, unpublished, and cross-CPO hubs return `404 hub_not_found`.

### 4.23 `DELETE /api/v1/app/favorite-hubs/{hub_id}`

Deletes only the current customer's composite hub favorite and returns `204`
whether it existed or was already absent. Cross-CPO and unknown resources are
not enumerated. A real removal records `CUSTOMER_FAVORITE_HUB_REMOVED`.

### 4.24 `PUT /api/v1/app/favorite-chargers/{charger_id}`

Uses `CustomerCharger.charger_id`, the six-character public charger ID; do not
use the internal UUID in `CustomerCharger.id`. The charger must be attached to
a published hub in the current CPO. The composite favorite is created
idempotently and returns `204`; a new row records
`CUSTOMER_FAVORITE_CHARGER_ADDED`. No HAL call occurs.

### 4.25 `DELETE /api/v1/app/favorite-chargers/{charger_id}`

Removes the current customer's charger favorite by `CustomerCharger.charger_id`
idempotently and returns `204`; the internal UUID is not accepted. An absent
or cross-CPO public ID is not enumerated. A real removal
records `CUSTOMER_FAVORITE_CHARGER_REMOVED`.

### 4.26 `GET /api/v1/app/hubs/{hub_id}/price`

Requires the authenticated customer bearer token and matching
`X-CPO-App-ID`. The hub must be published in the customer’s CPO. The server
selects the active tariff effective at the response’s `effective_at` timestamp.
For this hub-only route, charger-scoped candidates are not applicable, so the
order is matching UserGroup tariff, then hub tariff. In the current schema,
“UserGroup tariff” means the sole `user_group_id` target matching the
customer's existing group assignment.

`200 OK` returns `CustomerPriceResponse`. `AVAILABLE` contains exact decimal
strings for currency, `price_per_unit`, `tariff_type`, `price_type`, applicable
`units`, idle fee, and referenced active GST rates. A client must render the
price from its declared basis; it must not infer kWh from a field name.
`UNAVAILABLE` with `unavailable_reason: no_eligible_tariff` is returned when no
eligible tariff exists. `hub_gst_unavailable` is returned when the active hub
GST is missing or inactive; neither condition is substituted with a zero price
or zero tax. A zero-valued configured tariff and zero GST rates remain
`AVAILABLE`. The response is informational and is not a
charging or payment commitment. It does not contact HAL.

### 4.27 `GET /api/v1/app/chargers/{charger_id}/price`

Uses the six-character public charger ID and the same authentication, CPO,
publication, and response rules as the hub price route. The charger must be
attached to a published hub. The effective order is matching UserGroup tariff,
then charger tariff, then hub tariff. Every tariff has one target, so this
contains no composite-scope tie-breaker.

### 4.28 `GET /api/v1/app/chargers`

Requires the authenticated customer bearer token and matching
`X-CPO-App-ID`. Returns only chargers attached to published hubs in the
customer’s CPO. The safe projection includes hub display/location fields,
customer-safe charger display/category/parking metadata, connector summaries
with `connector_total_capacity`, static CMS administrative status for
chargers/connectors, and the customer's charger favorite flag. It excludes
OCPP identity, serial number, charger-host contact details, connection URLs,
and audit data. CMS `status` is not live availability; `availability` remains
`UNKNOWN` until HAL integration.

`CustomerHub.open_24_hours` is the hub's opening-hours value.
`CustomerCharger.twenty_four_seven_open_status` is the charger's own
opening-hours value, and `CustomerCharger.hub_open_24_hours` is the attached
hub's value. The values are independent; the `open_24_hours` query filter
selects by hub opening hours.

Optional filters are `q`, `connector_type`, `min_power_kw`, `max_power_kw`,
and `open_24_hours`. `q` searches the public charger ID, vendor, model, hub
name, and hub address. Supplying `lat` and `lng` enables near-me search within
`radius_km` (default 10 km, maximum 100 km), ordered by calculated distance.
Location searches are bounded and do not return a continuation cursor;
non-location searches use paired descending `before`/`before_id` keyset
pagination. `lat` and `lng` must be supplied together and `radius_km` must be
greater than zero. Invalid filters return `400 invalid_request`; authentication
and tenant errors use the standard app envelope.

Availability remains `UNKNOWN` for chargers and connectors until a separate
CMS/HAL contract is implemented. This route does not contact HAL.

### 4.28A `GET /api/v1/app/chargers/locations`

Requires the same authenticated customer bearer token and matching
`X-CPO-App-ID` as the full charger list. It accepts exactly the same optional
filters and cursor/near-me rules: `q`, `connector_type`, `min_power_kw`,
`max_power_kw`, `open_24_hours`, `limit`, paired `before`/`before_id`, and
`lat`/`lng`/`radius_km`. It returns the same current-CPO chargers attached to
published hubs, with the same ordering, pagination, and filter validation.

Each returned `chargers` item contains exactly `charger_name`, `latitude`, and
`longitude`. The coordinates are the attached hub's stored coordinates because
the charger record has no independent coordinate fields. The compact response
does not expose IDs, status, availability, connectors, favourites, pricing, or
other inventory detail. Invalid filters return the same `400` envelope as
4.28; authentication and tenant errors use the standard app envelope.

### 4.29 `GET /api/v1/app/wallet`

Requires the authenticated customer bearer token and matching
`X-CPO-App-ID`. Returns the wallet owned by that CPO-local customer:

```json
{
  "wallet": {
    "id": "5bd431a7-63f0-4df7-a2f5-1b55112df560",
    "balance": "1250.00",
    "currency": "INR",
    "updated_at": "2026-08-05T12:00:00Z"
  }
}
```

Balance is an exact decimal string. A missing wallet invariant returns
`404 wallet_not_found`; no customer or CPO ID is accepted from the request.
The route is read-only and does not perform recharge or provider operations.

### 4.30 `GET /api/v1/app/wallet/transactions`

Requires the authenticated customer bearer token and matching
`X-CPO-App-ID`. Returns the current customer’s wallet summary plus wallet
transactions in descending `(created_at, id)` keyset order. `limit` is 1–100
and defaults to 25; `before` and `before_id` must be supplied together. Each
transaction contains the durable amount, `CREDIT`/`DEBIT` type, description,
optional charging-session ID, financial status, and creation time. Internal
idempotency keys and provider credentials are never returned.

The route is read-only. Wallet recharge, refund, charging-session settlement,
and Razorpay provider verification are separate contracts. Errors include
`400 invalid_request`, `404 wallet_not_found`, standard authentication/app-ID
errors, or `500 internal_error`.

### 4.31 `POST /api/v1/app/wallet/recharge/orders`

Requires customer bearer authentication, matching `X-CPO-App-ID`, and an
`Idempotency-Key` header of 1–120 safe characters. The JSON body is:

```json
{"amount":"500.00"}
```

The amount must be a positive INR value with at most two decimal places. The
backend derives the wallet and CPO from the customer principal, creates a
durable recharge intent, resolves the encrypted CPO Razorpay credentials
internally, and creates the provider order through the Razorpay Go SDK. It
returns `201` with the internal recharge ID, Razorpay order ID, exact amount,
minor-unit amount, INR currency, `PAYMENT_PENDING` status, and public Razorpay
key ID. It never returns the key secret. Reusing the key with a different
amount returns `409 idempotency_conflict`; an ambiguous in-progress order
returns `409 recharge_order_pending` rather than creating an uncontrolled
duplicate provider order.

The operation stores a non-secret provider order snapshot and provider
metadata. It does not credit the wallet. If this CPO has an explicit expired
subscription, or its current subscription period has already elapsed, a new
order returns `403 cpo_subscription_expired`. A checkout order created before
expiry remains verifiable so a captured payment is never stranded.

### 4.32 `POST /api/v1/app/wallet/recharge/verify`

Requires customer bearer authentication and matching `X-CPO-App-ID`. The body
contains the three values returned by Razorpay Checkout:

```json
{
  "razorpay_order_id":"order_example",
  "razorpay_payment_id":"pay_example",
  "razorpay_signature":"<checkout-signature>"
}
```

The CMS verifies the signature with the internally resolved CPO key secret,
fetches the payment through the Razorpay SDK, and requires the provider order
ID, payment amount, currency, and captured status to match the durable recharge
intent. A captured payment is committed transactionally with its provider
snapshot, one completed wallet `CREDIT` transaction linked to the recharge
order, the wallet balance increment, and `PAID` order state. Repeated
verification is idempotent and cannot credit the same recharge twice.

Authorized-but-not-captured payments are stored without funding the wallet and
return `409 payment_not_captured`. Invalid signatures, provider mismatches,
missing orders, and provider failures use the standard error envelope.
Refund execution and webhook ingestion are not callable yet, but the durable
provider payment and future-refund records retain the IDs and non-secret
metadata needed for a later refund/reconciliation slice.

## 5. Authentication Workflow

### 5.1 `POST /api/v1/auth/login`

Purpose: validate a password and selected authority, then queue an email OTP.

Authentication: none.

Platform request:

```json
{
  "email": "superadmin@example.com",
  "password": "<current-password>",
  "scope": "PLATFORM"
}
```

CPO request:

```json
{
  "email": "admin@example.com",
  "password": "<current-password>",
  "scope": "CPO",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455"
}
```

Validation:

- email must parse as an email and is normalized to lowercase;
- password must be non-empty;
- scope is exactly `PLATFORM` or `CPO`;
- platform request must omit `cpo_id`;
- CPO request must include a non-zero CPO UUID;
- identity must be active and not currently locked;
- platform scope requires `platform_admins`;
- CPO scope requires active membership in an active CPO;
- mail must be enabled.

`202 Accepted`:

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
  "expires_at": "2026-07-23T12:10:00Z",
  "resend_available_at": "2026-07-23T12:01:00Z"
}
```

Durable effects:

- creates one `LOGIN_2FA` challenge containing only an OTP HMAC;
- invalidates earlier live challenges for the same purpose;
- inserts an encrypted `LOGIN_OTP` outbox job in the same transaction;
- clears failed-login state after a valid password and authority;
- failed passwords increment durable lockout state.

Errors:

- `400 invalid_request`: JSON contract failure;
- `401 invalid_credentials`: any invalid credential, identity, requested scope,
  CPO membership, CPO state, or platform authority;
- `429 rate_limited`: durable source-address limit exceeded;
- `503 mail_unavailable`: mail production disabled;
- `500 internal_error`.

Do not use the HTTP response to infer that SMTP has delivered the OTP. It means
the challenge and job were committed.

### 5.2 `POST /api/v1/auth/2fa/verify`

Purpose: consume the login challenge and create a durable session.

Authentication: none.

Request:

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
  "code": "123456"
}
```

Validation:

- challenge ID is a non-zero UUID;
- code is exactly six ASCII digits;
- challenge is LOGIN_2FA, unexpired, unconsumed, uninvalidated, and below its
  attempt limit;
- selected authority is revalidated transactionally.

`200 OK` for a platform session:

```json
{
  "access_token": "<encrypted-access-token>",
  "access_token_expires_at": "2026-07-23T12:15:00Z",
  "refresh_token": "<opaque-one-time-refresh-token>",
  "session_expires_at": "2026-08-22T12:00:00Z",
  "token_type": "Bearer",
  "must_change_password": false
}
```

`200 OK` for a CPO session:

```json
{
  "access_token": "<encrypted-access-token>",
  "access_token_expires_at": "2026-07-23T12:15:00Z",
  "refresh_token": "<opaque-one-time-refresh-token>",
  "session_expires_at": "2026-08-22T12:00:00Z",
  "token_type": "Bearer",
  "cpo_app_id": "cpo_dummy_735f36a898b84ce68a350db38c90bf9b",
  "cpo_app_id_mode": "DUMMY",
  "must_change_password": true
}
```

Durable effects:

- consumes the challenge;
- creates one scope-bound session;
- stores only the SHA-256 hash of the refresh token;
- marks the user verified and MFA-enabled;
- records last login and an audit event;
- queues a password-change reminder when required.

Errors: `400 invalid_request`, `401 invalid_challenge`,
`429 rate_limited`, or `500 internal_error`.

The refresh token is shown only in this response. Replace the stored value on
every successful refresh.

### 5.3 `POST /api/v1/auth/2fa/resend`

Purpose: invalidate an eligible login challenge and queue a replacement.

Request:

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497"
}
```

Eligibility:

- original challenge exists and is LOGIN_2FA;
- it is unconsumed, uninvalidated, and unexpired;
- `resend_available_at` has passed;
- authority is still valid;
- mail is enabled.

`202 Accepted` returns a new `ChallengeResponse`. The old challenge and OTP
become unusable.

Errors: `400 invalid_request`, `401 invalid_challenge`,
`429 rate_limited`, `503 mail_unavailable`, or `500 internal_error`.

### 5.4 `POST /api/v1/auth/refresh`

Purpose: rotate the opaque refresh token and issue a fresh access token.

Request:

```json
{
  "refresh_token": "<current-refresh-token>"
}
```

Validation:

- value is non-empty and at most 256 characters;
- token exists, is unused, unrevoked, and unexpired;
- session is active and unexpired;
- current identity and platform/CPO authority remain valid.

`200 OK` returns `TokenResponse`. The submitted refresh token is permanently
used; the returned replacement is now current. CPO responses contain the
current app ID, which allows client recovery after app-ID rotation.

Reuse handling: presenting a consumed refresh token revokes the entire session
and returns `401 invalid_refresh_token`.

Other errors: `400 invalid_request`, `429 rate_limited`, or
`500 internal_error`.

## 6. Password Operations

### 6.1 `POST /api/v1/auth/password/forgot`

Request:

```json
{"email":"admin@example.com"}
```

`202 Accepted`:

```json
{
  "message": "If the account is eligible, a password reset code will be sent."
}
```

Malformed email, unknown email, and eligible active identity intentionally
share that response. An eligible identity receives a single-use encrypted
`PASSWORD_RESET_OTP` job containing the opaque recovery ID (`challenge_id`),
six-digit code, and shared expiry. The API response intentionally contains none
of that challenge material, preserving enumeration safety.

Operational errors: `400 invalid_request`, `429 rate_limited`,
`503 mail_unavailable`, or `500 internal_error`.

### 6.2 `POST /api/v1/auth/password/reset`

Request:

```json
{
  "challenge_id": "988127b5-3954-4e46-9876-f90eeec5de26",
  "code": "123456",
  "new_password": "<10-to-128-character-password>"
}
```

`200 OK`:

```json
{"message":"Password reset. Sign in again."}
```

Success consumes the challenge, replaces the Argon2id hash, clears lockout and
`must_change_password`, and revokes every session and unused refresh token.
The frontend obtains both challenge ID and code from the eligible recipient's
email; it must not query internal storage. Pre-fix emails without a recovery ID
must be replaced by starting recovery again.

Errors:

- `400 invalid_request`;
- `400 invalid_password` with the exact length-policy explanation;
- `401 invalid_challenge`;
- `429 rate_limited`;
- `500 internal_error`.

### 6.3 `POST /api/v1/auth/password/change`

Authentication: bearer session. No app-ID header is required, allowing a
temporary-password user to complete the mandated change.

Request:

```json
{
  "current_password": "<current-password>",
  "new_password": "<10-to-128-character-password>"
}
```

`200 OK`:

```json
{
  "message": "Password changed. All sessions were revoked; sign in again."
}
```

Success clears `must_change_password` and revokes all sessions, including the
current bearer session.

Errors:

- `400 invalid_request`;
- `400 invalid_password`;
- `400 password_reused`;
- `401 unauthorized`;
- `401 invalid_current_password`;
- `500 internal_error`.

## 7. Identity and Sessions

### 7.1 `GET /api/v1/auth/me`

Authentication: bearer session.

Platform example:

```json
{
  "user": {
    "id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
    "email": "superadmin@example.com",
    "full_name": "Platform Superadmin",
    "is_verified": true,
    "mfa_enabled": true,
    "must_change_password": false,
    "last_login_at": "2026-07-23T12:00:00Z"
  },
  "scope": "PLATFORM"
}
```

CPO example adds:

```json
{
  "scope": "CPO",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "role": "ADMIN",
  "cpo_app_id": "cpo_dummy_735f36a898b84ce68a350db38c90bf9b",
  "cpo_app_id_mode": "DUMMY"
}
```

The actual response is one object containing `user` plus those scope fields.
Use it to bootstrap the authenticated UI and recover current app identity.

Errors: `401 unauthorized` or `500 internal_error`.

### 7.2 `GET /api/v1/auth/sessions`

Authentication: bearer session.

`200 OK`:

```json
{
  "sessions": [
    {
      "id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
      "scope": "CPO",
      "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "role": "ADMIN",
      "ip_address": "127.0.0.1",
      "user_agent": "Example Client",
      "created_at": "2026-07-23T12:00:00Z",
      "last_seen_at": "2026-07-23T12:05:00Z",
      "expires_at": "2026-08-22T12:00:00Z",
      "is_current": true
    }
  ]
}
```

Only active, unexpired sessions owned by the current global identity are
returned, newest first.

### 7.3 `DELETE /api/v1/auth/sessions/{session_id}`

Authentication: bearer session.

`204 No Content` revokes the selected owned session and its unused refresh
tokens. It may revoke the current session.

Errors:

- `400 invalid_request`: malformed UUID;
- `401 unauthorized`;
- `404 session_not_found`: missing or owned by another identity;
- `500 internal_error`.

### 7.4 `POST /api/v1/auth/logout`

Authentication: bearer session.

`204 No Content` revokes the current session and its unused refresh tokens.
The current access token becomes unusable on the next request.

### 7.5 `POST /api/v1/auth/logout-all`

Authentication: bearer session.

`204 No Content` revokes every platform and CPO session for the current global
identity, plus all unused refresh tokens.

## 8. Platform CPO Control Plane

All endpoints in this section require a bearer token whose durable session has
`PLATFORM` scope. `X-CPO-App-ID` is neither required nor accepted as tenant
authority.

### 8.1 `POST /api/v1/platform/cpos`

Request:

```json
{
  "slug": "example-charging",
  "business_name": "Example Charging Private Limited",
  "company_type": "COMPANY",
  "gstin": "19ABCDE1234F1Z5",
  "address": "1 Example Road",
  "city": "Kolkata",
  "state": "West Bengal",
  "pincode": "700001",
  "admin": {
    "email": "admin@example.com",
    "full_name": "CPO Administrator"
  }
}
```

Normalization and validation:

- `slug`: lowercase, at most 80 characters, lowercase words separated by one
  hyphen;
- `business_name`: required, at most 255;
- `company_type`: `INDIVIDUAL` or `COMPANY`;
- `gstin`: required, uppercased, exactly 15 uppercase letters/digits, and
  globally unique after normalization;
- `address`, `city`, `state`, and `pincode`: each required after trimming, with
  maxima 5000/100/100/10;
- admin email: normalized lowercase valid email, at most 320;
- admin full name: required, at most 255;
- status and app-ID fields cannot be supplied.

`201 Created`:

```json
{
  "cpo": {
    "id": "c821a013-5041-42f7-80c8-aa153cf9d455",
    "slug": "example-charging",
    "business_name": "Example Charging Private Limited",
    "company_type": "COMPANY",
    "gstin": "19ABCDE1234F1Z5",
    "address": "1 Example Road",
    "city": "Kolkata",
    "state": "West Bengal",
    "pincode": "700001",
    "status": "PENDING",
    "status_reason": "Initial provisioning",
    "status_changed_at": "2026-07-23T12:00:00Z",
    "status_changed_by_user_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
    "app_id": "cpo_dummy_735f36a898b84ce68a350db38c90bf9b",
    "app_id_mode": "DUMMY",
    "app_id_updated_at": "2026-07-23T12:00:00Z",
    "created_at": "2026-07-23T12:00:00Z",
    "updated_at": "2026-07-23T12:00:00Z"
  },
  "admin": {
    "user_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
    "email": "admin@example.com",
    "full_name": "CPO Administrator",
    "role": "ADMIN",
    "identity_created": true
  }
}
```

Transaction:

- serializes creation for the normalized admin email;
- creates pending CPO and unique dummy app ID;
- creates a new global identity and temporary-password hash, or locks/reuses an
  existing active identity without changing its password;
- creates active ADMIN membership;
- queues encrypted welcome/assignment email;
- writes audit record;
- commits all or none.

Errors:

- field-specific `400 invalid_*` codes listed in OpenAPI;
- `401 unauthorized`;
- `403 forbidden`;
- `409 cpo_slug_conflict` when the normalized slug already exists;
- `409 cpo_gstin_conflict` when the normalized GSTIN is assigned elsewhere;
- `409 cpo_app_id_conflict` for the generated app-ID collision;
- `409 admin_identity_conflict` when a concurrent request creates the global
  administrator identity first;
- `409 cpo_admin_membership_conflict` or `cpo_primary_admin_conflict` for
  administrator-membership races;
- `409 cpo_conflict` only as the safe fallback for an unrecognized unique
  constraint;
- `409 admin_identity_inactive`;
- `503 mail_unavailable`;
- `500 internal_error`.

The slug and GSTIN are enforced by normalized PostgreSQL unique indexes. GSTIN
must also pass the Indian structure and checksum rules, and its two-digit state
code must match the supplied CPO registration state; pincode must be a six-digit
Indian PIN code. PostgreSQL repeats those GSTIN/state/PIN invariants for direct
writes. Global GSTIN uniqueness already makes any `(GSTIN, business name)` pair
unique, so no redundant compound index exists. Legal business-name ownership is
not asserted without an authorized GST registry integration. The creation
transaction is authoritative even if an earlier availability lookup reported
that a slug was free.

### 8.2 `GET /api/v1/platform/cpos/slug-availability?slug={candidate}`

Purpose: validate and preflight a slug while the Superadmin fills the creation
form.

The required `slug` query value is trimmed and lowercased, then checked using
the same 80-character, single-hyphen-separated format as creation.

`200 OK`:

```json
{
  "slug": "example-charging",
  "available": true
}
```

The returned slug is the normalized value. `available` is only a current
snapshot: another request may create that slug immediately afterward. The FE
must still handle `409 cpo_slug_conflict` from CPO creation and must not treat
this GET as a reservation.

Errors: `400 invalid_slug`, `401 unauthorized`, `403 forbidden`, or
`500 internal_error`. The operation is read-only, side-effect-free, and safe to
retry.

### 8.3 `GET /api/v1/platform/cpos`

Purpose: drive the Superadmin CPO collection without loading an unbounded
tenant list.

Optional query:

- `q`: case-insensitive substring across business name, slug, GSTIN, app ID,
  primary-admin full name, and primary-admin email; at most 200 characters;
- `status`: exact `PENDING`, `ACTIVE`, or `SUSPENDED`;
- `app_id_mode`: exact `DUMMY` or `LIVE`;
- `limit`: 1 through 200, default 50;
- `before`: RFC3339 creation timestamp component of the exclusive
  newest-first cursor;
- `before_id`: UUID tie-breaker returned with `next_before`; it must be supplied
  together with `before`.

`200 OK`:

```json
{
  "cpos": [
    {
      "id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "slug": "example-charging",
      "business_name": "Example Charging Private Limited",
      "company_type": "COMPANY",
      "gstin": "19ABCDE1234F1Z5",
      "address": "1 Example Road",
      "city": "Kolkata",
      "state": "West Bengal",
      "pincode": "700001",
      "status": "PENDING",
      "status_reason": "Initial provisioning",
      "status_changed_at": "2026-07-31T09:00:00Z",
      "status_changed_by_user_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
      "app_id": "cpo_dummy_735f36a898b84ce68a350db38c90bf9b",
      "app_id_mode": "DUMMY",
      "app_id_updated_at": "2026-07-31T09:00:00Z",
      "created_at": "2026-07-31T09:00:00Z",
      "updated_at": "2026-07-31T09:00:00Z"
    }
  ],
  "next_before": "2026-07-31T09:00:00Z",
  "next_before_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "has_more": true
}
```

Ordering is newest creation time then descending UUID. If `has_more=true`, the
client sends both returned cursor fields unchanged. A changed filter/search
starts over without a cursor. The response contains no commercial-access data.

Errors: `400 invalid_q`, `invalid_status`, `invalid_app_id_mode`,
`invalid_limit`, `invalid_before`, `invalid_before_id`, or `invalid_cursor`;
shared authentication errors; or `500 internal_error`.

### 8.4 `GET /api/v1/platform/cpos/{cpo_id}`

Returns one current CPO object, including lifecycle reason/time/actor and app-ID
metadata. The object deliberately excludes primary-admin and secret-integration
data; use their owned resources.

Errors: `400 invalid_cpo_id`, `401 unauthorized`, `403 forbidden`,
`404 cpo_not_found`, or `500 internal_error`.

### 8.5 `PUT /api/v1/platform/cpos/{cpo_id}/profile`

Purpose: replace the mutable business profile while preserving stable CPO
identity, lifecycle, app identity, memberships, and tenant data.

```json
{
  "business_name": "Example Charging Limited",
  "company_type": "COMPANY",
  "gstin": "19ABCDE1234F1Z5",
  "address": "2 Example Road",
  "city": "Kolkata",
  "state": "West Bengal",
  "pincode": "700001"
}
```

All seven fields shown above are required. GSTIN is normalized uppercase, must
pass its Indian checksum, and must remain globally unique with a state code that
matches `state`; pincode must be a six-digit Indian PIN code. GSTIN, address,
city, state, and pincode cannot be null, blank, or omitted. The request cannot
mutate slug, status, app ID, CPO ID, or audit metadata.

The transaction updates the CPO, writes `CPO_PROFILE_UPDATED` audit evidence,
and emits `platform.cpo.profile_updated`. `200 OK` returns the updated CPO.

Errors: field-specific `400` errors from OpenAPI; shared authentication errors;
`404 cpo_not_found`; `409 cpo_gstin_conflict` for a GSTIN collision; or
`500 internal_error`.

### 8.6 `POST /api/v1/platform/cpos/{cpo_id}/activate`

Request:

```json
{"reason":"Approved after onboarding review"}
```

The trimmed reason must be 3–500 characters. The command stores `ACTIVE`,
reason, change time, and actor in the same transaction as audit action
`CPO_STATUS_ACTIVE` and event `platform.cpo.activated`. It does not create/check
commercial state and does not require a live app ID.

Calling it when already active returns current state without replacing the
original reason or duplicating audit/event evidence.

Errors: `400 invalid_request`, `invalid_reason`, or `invalid_cpo_id`; shared
authentication errors; `404 cpo_not_found`; or `500 internal_error`.

### 8.7 `POST /api/v1/platform/cpos/{cpo_id}/suspend`

Request:

```json
{"reason":"Access paused at operator request"}
```

The state transition stores reason/time/actor and emits the corresponding audit
and `platform.cpo.suspended` event atomically. It also revokes active CPO-staff
and customer sessions and their unused refresh tokens for that CPO. Platform
sessions have no CPO ID and are unaffected.

Repeated suspension does not duplicate lifecycle audit/event evidence, but it
still revokes any CPO sessions created since the original suspension.

Errors match activation.

### 8.8 `PUT /api/v1/platform/cpos/{cpo_id}/app-id`

Request:

```json
{"app_id":"example_charging_production"}
```

The server lowercases and trims the value. It must contain 16 to 100 lowercase
letters, digits, underscores, or hyphens and must not start with
`cpo_dummy_`.

`200 OK` returns the updated CPO with `app_id_mode: "LIVE"`. Rotation is
immediate. Existing sessions remain valid, while old app-ID headers fail.

Errors: `400 invalid_request`, `400 invalid_cpo_id`,
`400 invalid_cpo_app_id`, `401 unauthorized`, `403 forbidden`,
`404 cpo_not_found`, `409 cpo_app_id_conflict`, or `500 internal_error`.

### 8.9 `GET /api/v1/platform/cpos/{cpo_id}/primary-admin`

Purpose: provide the safe state needed by the Superadmin recovery UI.

`200 OK`:

```json
{
  "user_id": "e5288707-7266-44d4-b5a2-a87d06f1f2b7",
  "email": "admin@example.com",
  "full_name": "CPO Administrator",
  "role": "ADMIN",
  "membership_status": "ACTIVE",
  "identity_active": true,
  "identity_verified": true,
  "must_change_password": true,
  "latest_onboarding_delivery": {
    "job_id": "4ccb8733-b2e5-4f35-9953-f0e5f32176f2",
    "template": "CPO_STAFF_NEW_IDENTITY",
    "status": "PENDING",
    "attempts": 0,
    "created_at": "2026-07-31T09:00:00Z",
    "updated_at": "2026-07-31T09:00:00Z"
  }
}
```

`latest_onboarding_delivery` is absent when no correlated delivery exists.
This resource never exposes a password, OTP, encrypted payload, token, or SMTP
failure body.

Errors: `400 invalid_cpo_id`; shared authentication errors;
`404 primary_admin_not_found`; or `500 internal_error`.

### 8.10 `PUT /api/v1/platform/cpos/{cpo_id}/primary-admin`

Purpose: replace a departed primary administrator or restore the existing one.

```json
{
  "email": "replacement@example.com",
  "full_name": "Replacement Administrator",
  "reason": "Previous administrator left the organization"
}
```

Email is normalized lowercase and validated; name is 1–255 characters; reason
is 3–500 characters. Mail must be enabled.

The transaction is serialized for both CPO and email and guarantees at most one
primary membership:

- a new email creates a verified active identity with an Argon2id-hashed
  generated password and `must_change_password=true`; only its encrypted welcome
  job and rendered recipient email contain the temporary plaintext, and the
  transaction fails if that credential is absent from the welcome payload;
- an existing active identity is reused without changing password, name,
  verification state, or unrelated memberships;
- an inactive identity is rejected;
- the previous primary membership becomes `REVOKED` and non-primary;
- the previous primary's CPO sessions and refresh tokens for this CPO are
  revoked;
- the target membership is created, restored, or normalized to the currently
  supported `ADMIN` role;
- audit `CPO_PRIMARY_ADMIN_CHANGED`, event
  `platform.cpo.primary_admin_changed`, and applicable mail commit atomically.

Assigning the already-active current primary is a side-effect-free retry.
Assigning the same current identity after its membership was revoked restores
it and queues credential-free onboarding details.

`200 OK` returns the primary-admin view from endpoint 8.8.

Errors: request-field `400` errors from OpenAPI; shared authentication errors;
`404 cpo_not_found`; `409 admin_identity_inactive`,
`admin_identity_conflict`, `cpo_admin_membership_conflict`, or
`cpo_primary_admin_conflict`;
`503 mail_unavailable`; or `500 internal_error`.

### 8.11 `POST /api/v1/platform/cpos/{cpo_id}/primary-admin/resend-onboarding`

```json
{"reason":"Administrator requested access instructions again"}
```

The current identity and membership must both be active. The command queues a
correlated `CPO_ONBOARDING_RESENT` job containing CPO/app details and
working password-recovery guidance, audit action
`CPO_PRIMARY_ADMIN_ONBOARDING_RESENT`, and event
`platform.cpo.primary_admin_onboarding_resent` in one transaction. It never
reads, regenerates, or sends a password.

`202 Accepted` returns the primary-admin view including the new safe delivery
job.

Errors: `400 invalid_reason` or `invalid_cpo_id`; shared authentication errors;
`404 cpo_not_found` or `primary_admin_not_found`;
`409 primary_admin_unavailable`; `503 mail_unavailable`; or
`500 internal_error`.

### 8.12 `POST /api/v1/platform/cpos/{cpo_id}/administrative-sessions/revoke`

```json
{"reason":"Suspected credential exposure"}
```

`200 OK`:

```json
{
  "revoked_sessions": 3,
  "revoked_refresh_tokens": 2
}
```

Only active `CPO` sessions tied to this CPO and their unused refresh tokens are
revoked. It does not affect customer sessions, platform sessions, identities,
memberships, or tenant business data. The command is safe to repeat; even a
zero-count command records its reason/counts in audit action
`CPO_ADMIN_SESSIONS_REVOKED` and event
`platform.cpo.admin_sessions_revoked`.

Errors: `400 invalid_reason` or `invalid_cpo_id`; shared authentication errors;
`404 cpo_not_found`; or `500 internal_error`.

## 9. CPO Charging Network and Pricing

These routes are tenant business operations, not platform-superadmin routes.
Every request requires:

- an active CPO bearer session;
- `must_change_password=false`;
- the current `X-CPO-App-ID`;
- an active fixed CPO membership role.

The trusted CPO ID comes only from the verified session. No request body, path,
or header can select another CPO. A platform session cannot invoke these routes.

There is deliberately no mutable `/api/v1/cpo/profile` organization route. CPO
business/company details remain part of the platform-managed CPO record and are
changed only by a Superadmin through
`PUT /api/v1/platform/cpos/{cpo_id}/profile`. The CPO ADMIN can read a safe
projection through `GET /api/v1/cpo/organization`.

Every CPO business endpoint derives tenant context from a verified CPO session,
requires the matching `X-CPO-App-ID`, and evaluates the current active
membership plus fresh permission overrides. Roles are source-controlled default
bundles. `GET /api/v1/cpo/access/me` is the frontend authority projection;
explicit `DENY` wins over every role default, including ADMIN.

### 9.1 `GET /api/v1/cpo/organization`

Returns the authenticated CPO member's own CPO registration and operational
identity. The server obtains the CPO ID from the verified session; there is no
tenant path parameter or client-supplied scope.

```json
{
  "id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "slug": "example-charging",
  "business_name": "Example Charging Private Limited",
  "company_type": "COMPANY",
  "gstin": "19ABCDE1234F1Z5",
  "address": "12 Park Street",
  "city": "Kolkata",
  "state": "West Bengal",
  "pincode": "700016",
  "status": "ACTIVE",
  "status_changed_at": "2026-07-31T10:00:00Z",
  "app_id": "example_live_app_id",
  "app_id_mode": "LIVE",
  "app_id_updated_at": "2026-07-31T10:00:00Z",
  "created_at": "2026-07-30T10:00:00Z",
  "updated_at": "2026-07-31T10:00:00Z"
}
```

`gstin` and every address field are always present and nonblank. `app_id` is
the current non-secret application identifier the frontend sends as
`X-CPO-App-ID`.
Internal Superadmin actor IDs and the privileged lifecycle reason are omitted.
The endpoint is read-only, has no side effects, writes no audit event, and is
safe to retry. Organization changes remain Superadmin-only.

### 9.2 `GET /api/v1/cpo/admin/profile`

Returns the global identity profile used by the authenticated CPO member with
`organization.read`:

```json
{
  "user_id": "e5288707-7266-44d4-b5a2-a87d06f1f2b7",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "email": "admin@example.com",
  "full_name": "CPO Administrator",
  "phone": "+919876543210",
  "role": "ADMIN",
  "is_verified": true,
  "created_at": "2026-07-31T11:00:00Z",
  "updated_at": "2026-07-31T12:00:00Z"
}
```

`phone` is absent when unset. `/api/v1/auth/me` remains the canonical
authentication/bootstrap response and additionally carries session scope,
CPO/app IDs, and password-change state. This profile route does not return the
CPO organization profile.

### 9.3 `PATCH /api/v1/cpo/admin/profile`

Updates the global login identity used by the current CPO member with
`organization.manage`:

```json
{
  "full_name": "CPO Administrator",
  "phone": "+919876543210"
}
```

At least one field is required. Full name is trimmed, required when supplied,
and at most 255 characters. Phone is trimmed and at most 32 characters; a blank
phone clears it. Email, role, CPO membership, password, and verification state
cannot be changed here. Password changes remain under
`POST /api/v1/auth/password/change`.

The user update and `CPO_ADMIN_PROFILE_UPDATED` audit evidence commit
atomically. Audit details record field names only, not phone/name values.
`200 OK` returns the updated profile.

All create and update request bodies reject unknown properties, multiple JSON
objects, malformed JSON, and bodies over 32 KiB. Mutations are transactional
with their audit record. No route in this section contacts the OCPP HAL, creates
an OCPP socket, reports live connectivity, or delivers a remote command.

Shared middleware errors:

- `400 missing_cpo_app_id`;
- `401 unauthorized`;
- `403 forbidden`;
- `403 password_change_required`;
- `403 cpo_app_id_mismatch`;
- `500 internal_error`.

### 9.4 `GET /api/v1/cpo/users/{user_id}`

Returns a single basic administrative identity projection only when the UUID
has a staff membership in the authenticated CPO. Customer accounts are
separate and cannot be read through this route. This is a point lookup, not a
CPO staff or customer-directory API. The trusted CPO scope comes
from the verified ADMIN session, and an unlinked UUID returns the same
`404 user_not_found` response as an unknown UUID.

The response contains `id`, `cpo_id`, `email`, `full_name`, optional `phone`,
`is_active`, `is_verified`, timestamps, `role`, and `membership_status`. It
never returns customer accounts, credentials, sessions, OTPs, password metadata,
or a list of users. The read has no side effects or audit write.

Errors: `400 invalid_user_id`; shared authentication/authorization errors; or
`404 user_not_found`.

### 9.4A `GET /api/v1/cpo/wallet-transactions`

Returns wallet transaction projections for customers belonging to the
authenticated CPO. The read requires `customers.read` and current
`X-CPO-App-ID`; the CPO scope is derived from the verified session and cannot
be selected by the request. It is read-only, side-effect free, and does not
contact the HAL.

Optional query parameters:

- `limit`: 1–200 records; defaults to 50;
- `before`: RFC 3339 timestamp cursor;
- `before_id`: UUID tie-breaker used with `before`;
- `customer_id`: UUID filter for one CPO-local customer.

Results are ordered by `created_at DESC, id DESC`. When more rows exist,
`next_before` and `next_before_id` identify the final returned row for the
next request. Each transaction includes `id`, `customer_id`, customer name and
email, exact decimal `amount`, `currency`, `transaction_type`, `status`,
`created_at`, and nullable session/recharge-order IDs. `description` is
included when present.

```json
{
  "transactions": [
    {
      "id": "5bd431a7-63f0-4df7-a2f5-1b55112df560",
      "customer_id": "e5288707-7266-44d4-b5a2-a87d06f1f2b7",
      "customer_name": "Example Customer",
      "customer_email": "customer@example.com",
      "amount": "500.00",
      "currency": "INR",
      "transaction_type": "CREDIT",
      "status": "COMPLETED",
      "created_at": "2026-08-18T10:00:00Z"
    }
  ],
  "has_more": false
}
```

Invalid cursor, UUID, or limit values return `400 invalid_request`; missing or
invalid authentication/app-ID context returns the shared `401`/`403` errors.
Database failures return `500 internal_error`.

### 9.4B `GET /api/v1/cpo/analytics`

Returns aggregate tenant analytics for the authenticated CPO. The CPO scope
comes from the verified ADMIN session and cannot be selected by the request.
The read is side-effect free and does not contact the HAL.

```json
{
  "total_chargers": 12,
  "total_connectors": 24,
  "total_revenue": "18450.00",
  "total_usage": "921.500",
  "total_sessions": 87
}
```

`total_revenue` and `total_usage` are decimal strings. Revenue is the sum of
persisted session `total_amount`; usage is the sum of persisted session
`total_kwh`. Counts are non-negative integers and include only records owned by
the authenticated CPO. Errors are `401 unauthorized`, `403 forbidden`, or
`500 internal_error`.

### 9.4C `GET /api/v1/cpo/hubs/{hub_id}/analytics`

Returns aggregate analytics and the charger projections for one Hub belonging
to the authenticated CPO. The request requires the current CPO bearer and
`X-CPO-App-ID`; the CPO scope comes from the verified membership and cannot be
selected by the caller. A Hub belonging to another CPO is reported as
`404 hub_not_found`. The read is side-effect free and does not contact the HAL.

Optional query parameters:

- `period`: `day`, `week`, `month`, or `year`;
- `date`: ISO `YYYY-MM-DD` reference date for the selected period. For `day`,
  sessions started on that date are included; `week` uses the Monday–Sunday
  week containing the date; `month` uses the calendar month; and `year` uses
  the calendar year. The filter is applied only when both `period` and `date`
  are present; otherwise the aggregate is unfiltered.

The response contains `hub`, `analytics`, and `chargers`. `analytics` uses the
same decimal-string aggregates as the tenant route (`total_chargers`,
`total_connectors`, `total_revenue`, `total_usage`, and `total_sessions`).
`chargers` is the full CPO-scoped charger projection for the Hub, including its
connectors and live operational fields.

Malformed query binding returns `400 invalid_request`; shared `401`/`403`
authentication and capability failures, `404 hub_not_found`, and `500
internal_error` are also possible. Unsupported period values or malformed
dates currently result in an unfiltered aggregate.

### 9.5 `POST /api/v1/cpo/hubs`

Creates a commercial charging location. The server sources `id`, `cpo_id`, and
timestamps.

```json
{
  "name": "Park Street Hub",
  "address": "12 Park Street, Kolkata",
  "state": "West Bengal",
  "latitude": 22.5524,
  "longitude": 88.3521,
  "open_24_hours": true,
  "sanction_load": 120.5,
  "customer_visible": false,
  "charger_id": "7cc2d481-3ccb-4336-b03c-c8851a59ff9a"
}
```

Rules:

- `name`: required after trimming, 1–255 characters;
- `address`: required after trimming, 1–5000 characters;
- `state`: required after trimming, 1–100 characters;
- `latitude`: required, -90 through 90;
- `longitude`: required, -180 through 180;
- `open_24_hours`: optional, defaults to `true`;
- `sanction_load`: optional non-negative site electrical capacity in kW; it
  defaults to `0`, which means not recorded rather than zero capacity;
- `customer_visible`: optional publication switch, defaulting to `false`.
  Initial Hub creation must leave it false: `customer_visible:true` returns
  `409 hub_tariff_root_required` because the Hub must first exist, receive
  exactly one enabled unbounded Hub-root tariff, then be published through a
  Hub update route. It is not a live-availability claim;
- `charger_id`: optional UUID of an existing charger to assign to this hub.

`201 Created` returns:

```json
{
  "id": "8b80ef78-7799-4a09-a0d5-73ac944aa6e0",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "name": "Park Street Hub",
  "address": "12 Park Street, Kolkata",
  "state": "West Bengal",
  "latitude": 22.5524,
  "longitude": 88.3521,
  "open_24_hours": true,
  "sanction_load": 120.5,
  "customer_visible": false,
  "created_at": "2026-07-31T12:00:00Z",
  "updated_at": "2026-07-31T12:00:00Z"
}
```

The transaction writes audit action `HUB_CREATED`. Additional errors are
field-specific `400 invalid_*` responses and `409 hub_conflict`.

### 9.6 `GET /api/v1/cpo/hubs`

Returns hubs in descending `(created_at, id)` order. `limit` is 1–200 and
defaults to 50. When `has_more=true`, send both returned `next_before` and
`next_before_id` as `before` and `before_id` on the next request. The response:

```json
{
  "hubs": [],
  "next_before": "2026-07-31T12:00:00Z",
  "next_before_id": "8b80ef78-7799-4a09-a0d5-73ac944aa6e0",
  "has_more": true
}
```

Cursor fields are omitted when no next page exists. Errors:
`400 invalid_limit`, `400 invalid_before`, `400 invalid_before_id`, or
`400 invalid_cursor`.

### 9.7 `GET /api/v1/cpo/hubs/{hub_id}`

`hub_id` must be a non-zero UUID. The response returns the hub plus its
chargers in descending `(created_at, id)` order. `limit` is 1–200 and defaults
to 50. To continue, send `chargers.next_before` and
`chargers.next_before_id` together as `before` and `before_id`; both cursor
values are omitted when there is no next page.

```json
{
  "id": "8b80ef78-7799-4a09-a0d5-73ac944aa6e0",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "name": "Park Street Hub",
  "address": "12 Park Street, Kolkata",
  "state": "West Bengal",
  "latitude": 22.5524,
  "longitude": 88.3521,
  "open_24_hours": true,
  "sanction_load": 120.5,
  "customer_visible": false,
  "chargers": {
    "chargers": [],
    "has_more": false
  },
  "created_at": "2026-07-31T12:00:00Z",
  "updated_at": "2026-07-31T12:00:00Z"
}
```

A cross-tenant or unknown ID returns `404 hub_not_found`; malformed input or
an invalid cursor returns `400 invalid_hub_id`, `400 invalid_limit`,
`400 invalid_before`, `400 invalid_before_id`, or `400 invalid_cursor`.

### 9.8 `PATCH /api/v1/cpo/hubs/{hub_id}`

Accepts any non-empty subset of the seven create fields using the same
validation. Omitted fields are unchanged.

```json
{
  "address": "12A Park Street, Kolkata",
  "open_24_hours": false,
  "customer_visible": true
}
```

`200 OK` returns the updated Hub. The transaction writes `HUB_UPDATED` with
changed-field metadata. Additional errors: `400 invalid_hub`,
`400 invalid_hub_id`, `404 hub_not_found`, or `409 hub_conflict`.

### 9.8A `DELETE /api/v1/cpo/hubs/{hub_id}`

Takes no body. The request requires `hubs.manage`, locks the tenant hub, and
returns `204 No Content` after it transactionally unassigns its chargers and
removes User App hub and favorite links. Chargers themselves are retained.

If the same-CPO hub has tariffs, it returns `409 hub_has_tariffs` and changes
nothing. Unknown or cross-tenant hubs return `404 hub_not_found`; malformed
IDs return `400 invalid_hub_id`.

### 9.8B `PUT /api/v1/cpo/hubs/{hub_id}/customer-visibility`

Updates only the `hubs.manage` publication gate:

```json
{"customer_visible":true}
```

`200 OK` returns the updated hub and writes `HUB_CUSTOMER_VISIBILITY_UPDATED`.
When false, the hub and all attached chargers are omitted from User App network
discovery and favorite reads. This is a static publication decision, not live
availability. The route requires the same current CPO bearer session and
`X-CPO-App-ID` as every CPO business route.

### 9.9 `POST /api/v1/cpo/hubs/{hub_id}/chargers`

Assigns or reassigns one existing charger to the target hub. The request body
contains the charger's server-generated UUID:

```json
{
  "charger_id": "7cc2d481-3ccb-4336-b03c-c8851a59ff9a"
}
```

Both the target hub and charger must belong to the authenticated CPO. Chargers
may be created independently with no `hub_id`; this endpoint attaches one
later. Calling it with the charger's current hub is idempotent and creates no
new audit record. A change writes `CHARGER_HUB_REASSIGNED` with previous and
new hub metadata.

The relationship update is atomic. It does not move or rewrite tariffs:
charger-targeted tariffs remain on their charger and hub-targeted tariffs remain
on their hub. Errors include `400 invalid_hub_id` or `invalid_charger_id`, and
tenant-safe `404 hub_not_found` or `charger_not_found`.

### 9.9A `GET /api/v1/cpo/hubs/{hub_id}/chargers`

Returns all chargers attached to the same-CPO hub. The hub UUID is validated
against the authenticated ADMIN tenant scope; a cross-tenant or unknown hub
returns `404 hub_not_found`, and a malformed UUID returns `400
invalid_hub_id`. The response uses the standard `ChargerListResponse` envelope
with `has_more: false`; it has no side effects and does not contact the HAL.

Errors are `400 invalid_hub_id`, `401 unauthorized`, `403 forbidden`,
`404 hub_not_found`, or `500 internal_error`.

### 9.10 `POST /api/v1/cpo/chargers`

Creates one CMS charger projection and all initial connectors atomically.

The request should be sent as `multipart/form-data`. The main JSON payload should be in a form field named `data`, and the charger image should be in a file field named `charger_image`.

```http
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="data"

{
  "vendor": "Delta",
  "model": "DC Wallbox",
  "serial_number": "SN-001",
  "max_power_kw": 25,
  "charger_name": "My Charger",
  "charger_host_name": "Host Name",
  "charger_host_phone_no": "1234567890",
  "charger_type": "DC",
  "segment": "Public",
  "sub_segment": "Highway",
  "charger_use_type": "Commercial",
  "number_of_connectors": 1,
  "parking": "Covered",
  "protocol": "OCPP 1.6J",
  "twenty_four_seven_open_status": true,
  "connectors": [
    {
      "connector_number": 1,
      "connector_type": "CCS2",
      "connector_total_capacity": 25
    }
  ]
}
------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="charger_image"; filename="charger.jpg"
Content-Type: image/jpeg

(binary data)
------WebKitFormBoundary7MA4YWxkTrZu0gW--
```

Rules:

- `hub_id` is an optional UUID owned by this CPO; omit it to create an
  independent, unassigned charger;
- vendor and model are optional, trimmed when supplied, and at most 100
  characters each; serial number is required, trimmed, and at most 100
  characters;
- `max_power_kw` is optional/default zero and cannot be negative;
- All new charger fields are required.
- at least one connector is required;
- connector numbers are positive and unique within this request;
- connector type is required and at most 50 characters;
- `connector_total_capacity` is the create/update request field, is required
  on connector creation, and cannot be negative. Charger-level
  `total_capacity` is no longer part of the CMS charger contract; capacity is
  represented per connector and uses the same field name in CPO and User App
  connector projections.

The server generates:

- the charger row UUID;
- a globally unique six-character lowercase `charger_id`, also assigned as
  `ocpp_identity` for a newly created charger;
- each connector UUID;
- initial charger status `INACTIVE`;
- initial connector status `ACTIVE`;
- OCPP version projection `1.6J`;
- timestamps.

`201 Created` returns:

```json
{
  "id": "7cc2d481-3ccb-4336-b03c-c8851a59ff9a",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "charger_id": "a1b2c3",
  "ocpp_identity": "a1b2c3",
  "vendor": "Delta",
  "model": "DC Wallbox",
  "serial_number": "SN-001",
  "max_power_kw": 25,
  "status": "INACTIVE",
  "ocpp_version": "1.6J",
  "charger_name": "My Charger",
  "charger_host_name": "Host Name",
  "charger_host_phone_no": "1234567890",
  "charger_type": "DC",
  "segment": "Public",
  "sub_segment": "Highway",
  "charger_image": "/uploads/some-uuid.jpg",
  "charger_use_type": "Commercial",
  "number_of_connectors": 1,
  "parking": "Covered",
  "protocol": "OCPP 1.6J",
  "twenty_four_seven_open_status": true,
  "charger_connection_url_ws": "ws://localhost:8080/a1b2c3",
  "charger_connection_url_wss": "wss://localhost:8080/a1b2c3",
  "assigned": false,
  "connectors": [
    {
      "id": "540b3214-bd67-4f61-9134-ab462168fd92",
      "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "charger_id": "7cc2d481-3ccb-4336-b03c-c8851a59ff9a",
      "connector_number": 1,
      "connector_type": "CCS2",
      "connector_total_capacity": 25,
      "status": "ACTIVE",
      "created_at": "2026-07-31T12:05:00Z",
      "updated_at": "2026-07-31T12:05:00Z"
    }
  ],
  "created_at": "2026-07-31T12:05:00Z",
  "updated_at": "2026-07-31T12:05:00Z",
  "email": "admin@example.com"
}
```

`last_seen_at` is omitted until available. The transaction writes
`CHARGER_CREATED`. Additional errors: field-specific `400 invalid_*`,
`404 hub_not_found`, `409 charger_conflict`, or `409 connector_conflict`.

When unassigned, `hub_id` is omitted and `assigned` is `false`; after
attachment, `hub_id` is returned and `assigned` is `true`. `assigned` is a
server-generated response projection and is not accepted by create or update
requests. An independent charger cannot be used by a tariff until it is
assigned to the tariff's hub.

`ocpp_identity` is a CMS/HAL mapping value. Newly created chargers assign it
to the same generated value as `charger_id`; this does not register a charger
in the HAL or prove the charger is online. Older rows retain their existing
stored compatibility value and are not rewritten by this rule.

`charger_connection_url_ws` and `charger_connection_url_wss` are derived from
`CHARGER_CONNECTION_URL` and the stored OCPP identity without prefix rewriting;
they are connection addresses, not CMS API URLs. The deployment must provide the corresponding
WebSocket/TLS listener before a charger uses either address.

### 9.10A `PUT /api/v1/cpo/chargers/{charger_id}/customer-visibility`

Updates only the `chargers.manage` charger publication gate:

```json
{"customer_visible":true}
```

`200 OK` returns the updated charger and writes
`CHARGER_CUSTOMER_VISIBILITY_UPDATED`. A charger is included in User App
discovery, direct lookup, charger pricing, and favorite reads only when this
gate and its attached hub's `customer_visible` gate are both true. Existing
customer sessions and active charging are unaffected by unpublishing. The
route requires the same current CPO bearer session and `X-CPO-App-ID` as every
CPO business route.

The request body is required and contains the boolean `customer_visible`.
Unknown or cross-tenant chargers return `404 charger_not_found`; malformed
IDs or bodies return the shared `400` validation errors.

### 9.11 `GET /api/v1/cpo/chargers`

Returns tenant chargers and connectors in descending `(created_at, id)` order.
Each charger may also include an optional `live` projection containing the
committed CMS HAL connection state and connector availability/freshness. This
projection is read-only, CPO-scoped, and absent when no projection is
available; the request never calls HAL.

Query:

- `limit`: 1–200, default 50;
- `before`: exclusive RFC3339 timestamp from `next_before`;
- `before_id`: UUID tie-breaker from `next_before_id`.

Both cursor fields must be supplied together. `200 OK`:

```json
{
  "chargers": [
    {
      "id": "7cc2d481-3ccb-4336-b03c-c8851a59ff9a",
      "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "charger_id": "a1b2c3",
      "ocpp_identity": "a1b2c3",
      "vendor": "Delta",
      "model": "DC Wallbox",
      "serial_number": "SN-001",
      "max_power_kw": 25,
      "status": "INACTIVE",
      "ocpp_version": "1.6J",
      "charger_name": "My Charger",
      "charger_host_name": "Host Name",
      "charger_host_phone_no": "1234567890",
      "charger_type": "DC",
      "segment": "Public",
      "sub_segment": "Highway",
      "charger_image": "/uploads/some-uuid.jpg",
      "charger_use_type": "Commercial",
      "number_of_connectors": 1,
      "parking": "Covered",
      "protocol": "OCPP 1.6J",
      "twenty_four_seven_open_status": true,
      "charger_connection_url_ws": "ws://localhost:8080/CMS-4a58ce2df470b2b1",
      "charger_connection_url_wss": "wss://localhost:8080/CMS-4a58ce2df470b2b1",
      "assigned": false,
      "connectors": [
        {
          "id": "540b3214-bd67-4f61-9134-ab462168fd92",
          "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
          "charger_id": "7cc2d481-3ccb-4336-b03c-c8851a59ff9a",
          "connector_number": 1,
          "connector_type": "CCS2",
          "connector_total_capacity": 25,
          "status": "ACTIVE",
          "created_at": "2026-07-31T12:05:00Z",
          "updated_at": "2026-07-31T12:05:00Z"
        }
      ],
      "created_at": "2026-07-31T12:05:00Z",
      "updated_at": "2026-07-31T12:05:00Z",
      "email": "admin@example.com"
    }
  ],
  "next_before": "2026-07-31T12:05:00Z",
  "next_before_id": "7cc2d481-3ccb-4336-b03c-c8851a59ff9a",
  "has_more": true
}
```

The cursor fields are omitted when `has_more` is false. Errors:
`400 invalid_limit`, `400 invalid_before`, `400 invalid_before_id`, or
`400 invalid_cursor`.

### 9.12 `GET /api/v1/cpo/chargers/{charger_id}`

Uses the six-character public charger ID, not the charger UUID. Input is trimmed
and lowercased before validation. `200 OK` returns the Charger object including
connectors ordered by connector number and, when available, the optional
committed `live` projection. The response will also contain the `email` of the
CPO admin.
Unknown or cross-tenant IDs return `404 charger_not_found`; malformed IDs return `400 invalid_charger_id`.

### 9.12A `GET /api/v1/cpo/chargers/{charger_id}/image`

Uses the six-character public charger ID and returns the uploaded image as an
inline binary response for a charger owned by the caller's CPO. The response
sets the detected content type and supports byte ranges. A charger without an
uploaded file returns `404 image_not_found`; unknown or cross-tenant chargers
return `404 charger_not_found`.

### 9.13 `PATCH /api/v1/cpo/chargers/{charger_id}`

Updates any non-empty subset of the charger's properties. The request should be `multipart/form-data`, with the JSON payload in a form field named `data` and an optional new charger image in a file field named `charger_image`.

```json
{
  "hub_id": "8b80ef78-7799-4a09-a0d5-73ac944aa6e0",
  "vendor": "Delta",
  "model": "DC Wallbox V2",
  "serial_number": "SN-001",
  "max_power_kw": 30,
  "charger_name": "My Updated Charger",
  "connectors": [
    {
      "id": "540b3214-bd67-4f61-9134-ab462168fd92",
      "connector_number": 1,
      "connector_type": "CCS2",
      "connector_total_capacity": 30
    }
  ]
}
```

Each supplied connector must be an existing connector UUID on this charger and
must include at least one changed field. Connector IDs cannot repeat in one
request. New connectors can also be added by omitting the `id` field in the connector object.

This route does not change public
`charger_id`, `ocpp_identity`, charger or connector status, OCPP version, or
`last_seen_at`. Use the dedicated status route for the static CMS
administrative status; that status remains separate from future HAL live
availability.
If `hub_id` is supplied it assigns or reassigns the charger to that tenant hub;
the dedicated hub-assignment route provides the same operation with an
idempotent same-hub retry. A move does not alter tariff targets.

`200 OK` returns the updated Charger. The transaction writes `CHARGER_UPDATED`.
Additional errors include `404 charger_not_found`,
`404 connector_not_found`, `404 hub_not_found`, and uniqueness/reference
conflicts.

### 9.14 `DELETE /api/v1/cpo/chargers/{charger_id}`

Takes no body. It locks the charger, deletes its connectors and charger
transactionally, then writes `CHARGER_DELETED`. `204 No Content` means success.

PostgreSQL rejects deletion with `409 charger_in_use` while a tariff, charging
session, favorite, user-group access link, or another durable record references
the charger. The caller must explicitly remove or retire those dependent
records through their owning workflow; the API does not cascade business data.

### 9.14A `GET /api/v1/cpo/chargers/{charger_id}/status`

Uses the charger's internal UUID (`Charger.id`), not its six-character public
`Charger.charger_id`. Returns the CPO-owned static CMS administrative status
and stored OCPP identity mapping. It does not contact HAL and does not return
live charger availability.

### 9.14B `PUT /api/v1/cpo/chargers/{charger_id}/status`

Uses the same internal charger UUID. Request:

```json
{"status":"UNDERMAINTENANCE","ocpp_identity":"CP-001"}
```

`status` is required and must be one of `ACTIVE`, `INACTIVE`, `SUSPENDED`,
`UNDERMAINTENANCE`, or `DECOMMISSIONED`. `ocpp_identity` is optional; when
included it must exactly match the stored mapping or the request returns
`409 ocpp_identity_mismatch`. A successful update is CPO-scoped,
transactional, and writes `CHARGER_STATUS_UPDATED`.

### 9.15 `POST /api/v1/cpo/gsts`

Creates a named tenant GST profile.

```json
{
  "name": "Standard GST",
  "state": "West Bengal",
  "sgst_rate": "9.00",
  "cgst_rate": "9.00",
  "igst_rate": "0.00",
  "is_active": true
}
```

All five non-boolean fields are required. The name and state are trimmed and
limited to 100 characters. Each exact decimal rate is 0–100 inclusive. Decimal JSON
strings are recommended; JSON numbers are also accepted. `is_active` defaults
to true. The normalized name is unique per CPO.

Migration thirty-one permits pre-existing GST rows to retain null rate values;
those response fields are omitted when null. New profiles cannot omit rates.
A profile cannot mix non-zero split (SGST/CGST) and integrated (IGST) tax.

`201 Created` returns the generated UUID, trusted CPO ID, exact decimal rates
as JSON strings, active state, and timestamps. The transaction writes
`GST_CREATED`. Additional errors: field-specific `400 invalid_*` and
`409 gst_conflict`.

### 9.16 `GET /api/v1/cpo/gsts`

Returns bounded GST pages using the same `limit`, `before`, `before_id`,
`next_before`, `next_before_id`, and `has_more` semantics as hub listing:

```json
{
  "gsts": [],
  "has_more": false
}
```

Both cursor inputs are required together.

### 9.17 `GET /api/v1/cpo/gsts/{gst_id}`

Returns one GST profile by server-generated UUID. Cross-tenant and unknown IDs
return `404 gst_not_found`; malformed UUIDs return `400 invalid_gst_id`.

### 9.18 `PATCH /api/v1/cpo/gsts/{gst_id}`

Accepts any non-empty subset of `name`, `state`, `sgst_rate`, `cgst_rate`,
`igst_rate`, and `is_active`, using the create validation for supplied fields.
Omission preserves a field. The service applies the patch to the complete GST
profile under a tenant commercial lock and rejects it if the result would make
an assigned Hub relationship invalid; deactivate or change a profile only
after unassigning it when necessary. `200 OK` returns the updated GST profile
and writes `GST_UPDATED`.

There is currently no GST delete route. An inactive profile remains durable for
historical references.

Hub GST assignment uses `POST` and `PATCH
/api/v1/cpo/hubs/{hub_id}/gst` with `{"gst_id":"<uuid>"}`. The referenced GST
must belong to the same CPO and cannot already be assigned to another hub. The
hub and GST states determine the applicable rates: same-state assignment
requires present SGST and CGST components and zero IGST; different-state
assignment requires a present IGST component and zero SGST/CGST. Zero is a
valid persisted value for a non-applicable component. The same full validation
also protects replacement, GST state/rate/activation changes, and Hub state
changes under locking. Invalid combinations return `400 invalid_gst_for_hub`.
`GET` reads the assigned profile and `DELETE` unassigns it.

### 9.19 Scoped tariff routes

All tariff routes require the current `X-CPO-App-ID`; reads require
`tariffs.read` and mutations require `tariffs.manage`. Tariffs are created,
listed, read, updated, and deleted through one scope:

- hub: `POST`/`GET /api/v1/cpo/hubs/{hub_id}/tariffs` and
  `GET`/`PATCH`/`DELETE /api/v1/cpo/hubs/{hub_id}/tariffs/{tariff_id}`;
- charger: `POST`/`GET /api/v1/cpo/chargers/{charger_id}/tariffs` and
  `GET`/`PATCH`/`DELETE /api/v1/cpo/chargers/{charger_id}/tariffs/{tariff_id}`;
- user group: `POST`/`GET /api/v1/cpo/user-groups/{user_group_id}/tariffs`
  and `GET`/`PATCH`/`DELETE /api/v1/cpo/user-groups/{user_group_id}/tariffs/{tariff_id}`.

The path scope is part of the tenant ownership lookup and is the only source of
the tariff target. Every tariff has **exactly one** durable target:
`assigned_to=hub` with only `hub_id`, `assigned_to=charger` with only
`charger_id`, or `assigned_to=usergroup` with only `user_group_id`. A charger
tariff may target an independent charger; it does not require hub context.
All target records must belong to the authenticated CPO.

Create requests use this body:

```json
{
  "price_per_unit": "16.9100",
  "idle_fee_per_min": "0.0000",
  "currency": "INR",
  "tariff_type": "fixed",
  "price_type": "energy",
  "units": "kwh",
  "is_active": true,
  "start_date": "2026-09-01T00:00:00Z",
  "end_date": "2026-10-01T00:00:00Z"
}
```

Rules:

- the nested hub, charger, or user-group URL determines the target; target IDs
  in a JSON body are rejected as unknown fields;
- tariffs are commercial-only. GST is assigned to a hub through the hub GST
  routes and is not accepted in tariff create or update requests;
- `PATCH` updates commercial fields only and cannot move a tariff between
  target types or target records;
- `price_per_unit` is required and may be zero for free charging;
- idle fee is optional/default zero and cannot be negative. A non-zero idle
  fee is rejected for an active tariff because the CMS has no authoritative
  idle interval and does not bill idle time; zero remains a retained durable
  field for snapshots and inactive legacy records;
- currency is optional/default `INR`, normalized uppercase, and exactly three
  letters;
- `tariff_type` is required and currently must be `fixed`;
- `price_type` is required: `energy` bills measured Wh divided by 1000 at the
  exact declared per-kWh price, `time` bills actual HAL
  StartTransaction-to-StopTransaction minutes, and `sessions` bills one
  completed session regardless of measured energy or duration;
- `units` is required as `kwh` for energy, required as `minutes` for time,
  and omitted for sessions;
- any other combination fails with `unsupported_tariff_pricing` before
  persistence;
- `is_active` is optional/default true;
- schedule shape is one of: a root (`start_date` and `end_date` both omitted),
  an open-ended fallback (`start_date` supplied and `end_date` omitted), or a
  bounded override (`start_date < end_date`) active on `[start_date, end_date)`.
  An end-only schedule is invalid;
- date semantics are exact instants, not charging-session duration limits. No
  worker changes a row when time passes; `is_active` means administratively
  enabled only;
- among enabled tariffs for one exact target, there is at most one root,
  open-ended fallbacks have unique starts, and bounded overlap is allowed only
  for strict containment. Crossing or identical bounded intervals return
  `409 tariff_temporal_conflict`. Adjacent intervals where `end_date ==
  start_date` do not overlap;
- at an instant, the target chooses its deepest matching bounded override, else
  its newest started open fallback, else its root. This temporal choice happens
  only after scope precedence: UserGroup > Charger > Hub. A UserGroup root
  therefore wins over any Charger or Hub temporal override.

Exact decimal strings are recommended. `201 Created` returns the generated
tariff UUID, relations, exact decimal strings, currency, active state, and
timestamps. Scope-specific creation writes `HUB_TARIFF_CREATED`,
`CHARGER_TARIFF_CREATED`, or `USER_GROUP_TARIFF_CREATED`.

Errors include field-specific `400 invalid_*`, relation-specific `404`
responses, `409 tariff_conflict`, `409 tariff_temporal_conflict`, and, for a
published Hub floor violation, `409 hub_tariff_root_required`.

Each scoped `GET` collection returns bounded keyset pages:

```json
{
  "tariffs": [],
  "has_more": false
}
```

Every row belongs to the authenticated CPO and the path scope. Current listing
returns active and inactive tariffs.

A scoped detail `GET` returns `404 tariff_not_found` for an unknown or
cross-scope tariff; malformed UUIDs return the applicable `400 invalid_*_id`
error.

A scoped `PATCH` accepts any non-empty subset of the commercial create fields.
Every omitted key leaves its stored value unchanged. `units`, `start_date`, and
`end_date` additionally accept explicit JSON `null`: `units:null` clears units
for a `sessions` tariff, and `start_date:null` together with `end_date:null`
clears a prior schedule to the unbounded root. A supplied `start_date` with an
omitted or null `end_date` creates an open-ended fallback. A supplied end date
requires a start date; the resulting schedule is root, open fallback, or a
complete half-open bounded interval. The server applies the request to the locked stored row and
then validates the complete resulting tariff, so a basis change must include
the complete intended combination (`energy`/`kwh`, `time`/`minutes`, or
`sessions` with no units). `watt/hour` is historical snapshot compatibility
only and is never accepted for a current tariff.

For example, change an energy tariff to sessions with:

```json
{"tariff_type":"fixed","price_type":"sessions","units":null}
```

Clear an existing schedule with:

```json
{"start_date":null,"end_date":null}
```

GST remains outside tariff PATCH; Hub GST assignment owns it. `200 OK` returns
the updated tariff and writes the matching scope-specific update audit event.
Errors match creation plus `404 tariff_not_found`.

`DELETE` returns `204 No Content` for the same nested target path. It never
moves a tariff or deletes charging history: a foreign-key reference to frozen
pricing, start intent, or charging history returns `409 tariff_in_use`. Deleting
or deactivating the sole active root on a customer-visible Hub returns
`409 hub_tariff_root_required`; deleting any row also validates the resulting
enabled temporal topology. Deactivation remains suitable when historical policy
retention is desired.

### CPO Settings

`GET /api/v1/cpo/settings` returns the settings for the authenticated CPO member
with `settings.read`.
The CPO is derived from the verified session and `X-CPO-App-ID`; callers cannot
select another tenant. CPO provisioning creates a blank settings row, while
migration forty-three backfills existing CPOs, so a normal CPO GET returns the
zero-default wallet policy rather than requiring a preliminary settings write.

`POST` and `PUT /api/v1/cpo/settings` accept `multipart/form-data` with the
following optional fields:

- `invoice_note`: text retained with the CPO settings;
- `invoice_logo`: an uploaded logo file, stored under the service `uploads`
  directory and returned as the stored relative path.
- `wallet_min_balance`: optional non-negative whole-currency balance required
  before a customer can start a charging session; default `0`.
- `wallet_buffer_min_balance`: optional non-negative whole-currency amount
  withheld from the usable balance at each session start; default `0`.

The update is tenant-scoped and upserts the single settings row for the CPO.
Omitting any field preserves its existing value. Successful requests return the
JSON settings projection containing both non-null integer wallet fields plus
the optional `invoice_logo` and `invoice_note` fields. During each new start,
the backend locks the wallet, requires `balance >= wallet_min_balance`, and
calculates tariff affordability from `balance - wallet_buffer_min_balance`.
Errors include the shared authentication and forbidden responses,
`400 invalid_wallet_min_balance`, `400 invalid_wallet_buffer_min_balance`, or
`500 internal_error`.

`GET /api/v1/cpo/settings/invoice-logo` streams the authenticated CPO's stored
logo inline. It accepts no path or CPO parameter and responds with byte-range
support, a detected image content type, and `X-Content-Type-Options: nosniff`.
It returns `404 settings_not_found` if the CPO has no settings row and
`404 invoice_logo_not_found` if no logo is stored or the referenced file is
absent.

### 9.20 User groups

`POST`/`GET /api/v1/cpo/user-groups` creates or lists the authenticated CPO's
user groups using the usual bounded keyset pagination. A create body has
required non-empty `name`, optional `description`, and optional `is_active`
(default true). A duplicate-name conflict returns `409 user_group_conflict`.

`GET`/`PATCH`/`DELETE /api/v1/cpo/user-groups/{user_group_id}` reads, updates,
or removes one CPO-owned user group. `PATCH` accepts a non-empty subset of
`name`, `description`, and `is_active`. Unknown and cross-tenant IDs return
`404 user_group_not_found`; malformed IDs return `400 invalid_user_group_id`.
Deletion returns `204 No Content` and returns `409 user_group_in_use` when a
tariff still references the group.

The detail `GET` response includes a `members` array containing safe CPO
customer projections for customers currently assigned to that group. The
array is omitted when it is empty; each member includes the same
`usergroup_assigned` boolean exposed by the CPO customer directory.

`POST /api/v1/cpo/user-groups/{user_group_id}/members` assigns a same-CPO
customer using `{"customer_id":"<uuid>"}`. Assigning a customer already in
that group is idempotent and returns `204`; a customer assigned to another
group returns `409 customer_already_in_group`. Unknown or cross-tenant group
or customer IDs return the corresponding `404` error.

`DELETE /api/v1/cpo/user-groups/{user_group_id}/members/{customer_id}` removes
that same-CPO customer from the specified group. Removing a customer who is
already absent is idempotent and returns `204`; the group and customer are
still validated for tenant ownership before the no-op result. Both commands
write tenant audit evidence only when membership changes.

### 9.23 `GET /api/v1/cpo/subscription`

Returns the current non-terminal subscription details for the authenticated
CPO. `CANCELLED` and `EXPIRED` history is deliberately not returned by this
tenant read.

`200 OK`:

```json
{
  "id": "a4d3b5b6-4a4a-4a4a-4a4a-5b4b3b2b1b0a",
  "status": "ACTIVE",
  "starts_at": "2026-07-01T00:00:00Z",
  "current_period_starts_at": "2026-08-01T00:00:00Z",
  "current_period_ends_at": "2026-09-01T00:00:00Z",
  "cancel_at_period_end": false,
  "plan": {
    "name": "Pro Plan",
    "description": "Full-featured plan for large CPOs.",
    "currency": "INR",
    "price_minor": 1000000,
    "billing_interval": "MONTHLY",
    "interval_count": 1,
    "trial_days": 0
  }
}
```

The response includes the current subscription status and period, along with
details of the subscribed plan. Optional fields such as `trial_ends_at`,
`cancelled_at`, and `ended_at` are omitted when absent. This is read-only:
manual issue and lifecycle commands remain platform-Superadmin operations, and
subscription state never controls CPO authorization.

Errors: `401 unauthorized`, `403 forbidden`, `404 subscription_not_found`, or `500 internal_error`.

### 9.24 CPO Customer Directory

These read-only endpoints allow a CPO member with `customers.read` to view the
app customers registered under their CPO. They require the standard CPO bearer session and
matching `X-CPO-App-ID`. The CPO is derived from the session; no request
parameter can select another tenant's customers.

#### 9.24.1 `GET /api/v1/cpo/customers`

Returns a paginated list of customers for the authenticated CPO, ordered by
newest first.

Query parameters:

- `q`: case-insensitive substring search across customer name, email, and
  phone; at most 200 characters.
- `status`: exact `ACTIVE` or `BLOCKED`.
- `limit`: 1 through 200, default 50.
- `before`: RFC3339 creation timestamp for keyset pagination.
- `before_id`: UUID tie-breaker to be paired with `before`.

The response contains a list of safe customer projections, omitting wallet and
credential details, plus the standard `next_before`, `next_before_id`, and
`has_more` cursor fields. Each customer projection includes `usergroup_assigned`,
which is true when that customer currently belongs to any user group in the
authenticated CPO.

Errors: `400 invalid_q`, `invalid_status`, `invalid_limit`, `invalid_cursor`;
shared authentication errors; or `500 internal_error`.

<details>
<summary>OpenAPI Specification</summary>

```yaml
paths:
  /api/v1/cpo/customers:
    get:
      tags:
        - "CPO Operations - Account & Notifications"
      summary: "List CPO customers"
      description: "Returns a paginated list of customers for the authenticated CPO, ordered by newest first."
      produces:
        - "application/json"
      parameters:
        - name: "q"
          in: "query"
          description: "Case-insensitive substring search across customer name, email, and phone"
          required: false
          type: "string"
          maxLength: 200
        - name: "status"
          in: "query"
          description: "Filter by customer status (ACTIVE or BLOCKED)"
          required: false
          type: "string"
        - name: "limit"
          in: "query"
          description: "Number of records to return (1-200, default 50)"
          required: false
          type: "integer"
        - name: "before"
          in: "query"
          description: "RFC3339 creation timestamp for keyset pagination"
          required: false
          type: "string"
          format: "date-time"
        - name: "before_id"
          in: "query"
          description: "UUID tie-breaker to be paired with before"
          required: false
          type: "string"
          format: "uuid"
      responses:
        "200":
          description: "Successfully retrieved customer list"
        "400":
          description: "Invalid query parameters"
        "401":
          description: "Unauthorized"
        "403":
          description: "Forbidden"
        "500":
          description: "Internal server error"
      security:
        - BearerAuth: []
        - CPOAppID: []
```

</details>

#### 9.24.2 `GET /api/v1/cpo/customers/{customer_id}`

Returns a single customer belonging to the authenticated CPO. A cross-tenant
or unknown ID returns `404 customer_not_found`.

The response is a safe projection:

```json
{
  "id": "e8a751ff-d7d4-4ce8-ab30-cdd8c8111363",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
  "email": "driver@example.com",
  "full_name": "Example Driver",
  "phone": "+919876543210",
  "status": "ACTIVE",
  "is_verified": true,
  "usergroup_assigned": false,
  "created_at": "2026-07-23T12:00:00Z",
  "last_login_at": "2026-07-23T12:05:00Z"
}
```

## 10. CPO Integration Credentials

All endpoints require:

- bearer CPO session;
- role `ADMIN`;
- `must_change_password=false`;
- current `X-CPO-App-ID`.

Only provider `RAZORPAY` is supported. Provider input is normalized uppercase.
There is no secret-read endpoint.

Shared middleware failures:

- `400 missing_cpo_app_id`;
- `401 unauthorized`;
- `403 forbidden`;
- `403 password_change_required`;
- `403 cpo_app_id_mismatch`.

### 10.1 `GET /api/v1/cpo/integrations`

`200 OK`:

```json
{
  "integrations": [
    {
      "provider": "RAZORPAY",
      "display_hint": "****5678",
      "is_active": true,
      "configured_at": "2026-07-23T12:00:00Z",
      "updated_at": "2026-07-23T12:00:00Z"
    }
  ]
}
```

Returns only rows for the authenticated CPO.

### 10.2 `GET /api/v1/cpo/integrations/{provider}`

Returns the same metadata object.

Additional errors:

- `400 unsupported_integration_provider`;
- `404 integration_not_found`.

### 10.3 `PUT /api/v1/cpo/integrations/{provider}`

Request:

```json
{
  "key_id": "<8-to-100-character-key-id>",
  "key_secret": "<16-to-255-character-secret>",
  "webhook_secret": "<optional-up-to-255-character-secret>"
}
```

The server trims all fields, validates lengths, encrypts the complete JSON
object with CPO/provider-bound AES-256-GCM, and atomically creates or replaces
the row. Audit data contains provider only.

`200 OK` returns metadata, never the submitted fields:

```json
{
  "provider": "RAZORPAY",
  "display_hint": "****5678",
  "is_active": true,
  "configured_at": "2026-07-23T12:00:00Z",
  "updated_at": "2026-07-23T12:00:00Z"
}
```

Additional errors: `400 invalid_request`,
`400 unsupported_integration_provider`, and
`400 invalid_integration_credentials`.

### 10.4 `DELETE /api/v1/cpo/integrations/{provider}`

`204 No Content` deletes the encrypted row and records an audit event.

Additional errors: `400 unsupported_integration_provider` and
`404 integration_not_found`.

## 11. Platform Operations, Audit, Workers, and Realtime

Every endpoint in this section requires an active bearer token whose durable
session still resolves to `PLATFORM`. CPO and customer sessions receive `403
forbidden`. The routes never accept `X-CPO-App-ID` as authority and do not
grant access to tenant business records.

### 11.1 `GET /api/v1/platform/events`

Purpose: replay committed control-plane facts for UI invalidation and recovery.
This endpoint is the durable REST fallback for the SSE stream.

Query:

- `after_id`: optional integer greater than or equal to zero; returns IDs
  strictly greater than this cursor;
- `limit`: optional integer from 1 through 500, default 50;
- `type`: optional exact event-type match, at most 150 characters.

`200 OK`:

```json
{
  "events": [
    {
      "id": 14582,
      "type": "platform.cpo.suspended",
      "actor_user_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
      "resource_type": "CPO",
      "resource_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "data": {"status": "SUSPENDED"},
      "occurred_at": "2026-07-24T12:14:52Z"
    }
  ],
  "next_cursor": 14582,
  "has_more": false
}
```

Events are ordered by ascending ID and delivered at least once across retries.
Clients persist `next_cursor`, deduplicate by `id`, and refresh the affected
REST resource rather than treating event data as authoritative state. An event
older than retention may no longer be replayable.

Errors:

- `400 invalid_after_id`, `invalid_limit`, or `invalid_type`;
- `401 unauthorized`;
- `403 forbidden`;
- `409 realtime_cursor_expired`: discard the cursor, refresh the relevant REST
  snapshots, then resume from the current retained event range;
- `500 internal_error`.

### 11.2 `GET /api/v1/platform/realtime/stream`

Purpose: deliver the same durable platform-event log as server-sent events.
The response is `text/event-stream`.

Resume input:

- `Last-Event-ID` header: preferred last processed numeric event ID;
- `after_id`: query fallback when that header is absent;
- `limit`: initial replay batch size from 1 through 500, default 50;
- `type`: optional exact event-type filter.

The bearer token must remain in the `Authorization` header. Browser clients
therefore use authenticated `fetch` streaming; tokens must never be placed in
the URL. Each fact is framed as:

```text
id: 14582
event: platform.cpo.suspended
data: {"id":14582,"type":"platform.cpo.suspended","resource_type":"CPO","resource_id":"c821a013-5041-42f7-80c8-aa153cf9d455","data":{"status":"SUSPENDED"},"occurred_at":"2026-07-24T12:14:52Z"}
```

Heartbeat comments keep the connection active and revalidate the durable
session. Logout, session revocation, authority removal, network failure, or
server shutdown closes the stream. The client reconnects with its last
processed ID and uses endpoint 11.1 plus normal resource APIs for missed-event
recovery. Ordering is ascending event ID; duplicate delivery is possible.

Before streaming begins, errors use the same JSON envelope and status codes as
endpoint 11.1. After streaming begins, errors close the connection because an
HTTP status can no longer be replaced.

### 11.3 `GET /api/v1/platform/audit-logs`

Purpose: query immutable privileged-action evidence. Audit records and realtime
events serve different purposes: audit is security evidence, while events are
retention-bounded UI/recovery notifications.

Filters:

- `before`: RFC3339 timestamp component of an exclusive newest-first cursor;
- `before_id`: UUID tie-breaker returned with `next_before`; it is invalid
  without `before`;
- `limit`: 1 through 500, default 50;
- `action`: exact action, at most 100 characters;
- `entity`: exact entity type, at most 100 characters;
- `actor_user_id`: exact actor UUID;
- `cpo_id`: exact affected CPO UUID.

`200 OK`:

```json
{
  "records": [
    {
      "id": "d77ade2e-3fd2-48b1-9383-cfb9375a659a",
      "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "user_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
      "action": "CPO_SUSPENDED",
      "entity": "CPO",
      "entity_id": "c821a013-5041-42f7-80c8-aa153cf9d455",
      "details": {"previous_status": "ACTIVE", "status": "SUSPENDED"},
      "created_at": "2026-07-24T12:14:52Z"
    }
  ],
  "next_before": "2026-07-24T12:14:52Z",
  "next_before_id": "d77ade2e-3fd2-48b1-9383-cfb9375a659a",
  "has_more": true
}
```

When `has_more` is true, the next request sends both cursor fields. This pair
prevents records sharing one timestamp from being skipped. Audit details are
sanitized metadata and never contain credentials, OTPs, tokens, or mail
payloads.

Errors: `400 invalid_before`, `invalid_before_id`, `invalid_limit`,
`invalid_action`, `invalid_entity`, `invalid_actor_user_id`, or
`invalid_cpo_id`; shared authentication errors; or `500 internal_error`.

### 11.4 `GET /api/v1/platform/workers`

Purpose: show the durable current-health projection for each logical worker.

`200 OK`:

```json
{
  "workers": [
    {
      "id": "888360ea-15fb-459b-9fb0-12624a81f011",
      "name": "mail-outbox",
      "instance_key": "ee16b752-6e12-49fe-b685-cf1431cadf7a",
      "status": "HEALTHY",
      "required": true,
      "started_at": "2026-07-24T12:00:00Z",
      "last_heartbeat_at": "2026-07-24T12:14:50Z",
      "last_job_completed_at": "2026-07-24T12:14:48Z",
      "metadata": {}
    }
  ]
}
```

`name` is the logical worker role and `instance_key` identifies its current
process incarnation. Historical process rows are retained for operations but
are intentionally absent from this current-status endpoint. A replacement
heartbeat atomically supersedes the old instance; a delayed old heartbeat
cannot reclaim current state.

`STALE` is derived at read time when the current heartbeat exceeds
`PLATFORM_WORKER_STALE_AFTER`; it is not a separately reported database state.
Required current workers that are stale or report a non-healthy state make
`GET /health/ready` return `503`. This endpoint is observational only: it
cannot start, stop, restart, or kill a process.

Errors: shared `401`, `403`, or `500` responses.

### 11.5 `POST /api/v1/platform/hal-facts/{fact_id}/requeue`

Purpose: request recovery of one exact HAL fact after HAL has stopped automatic
delivery in `RECONCILIATION_REQUIRED`. A current `PLATFORM` bearer session is
required. `fact_id` is a non-zero UUID. The CMS supplies its server-generated
request UUID as the HAL correlation header; clients do not choose it.

The command reaches only HAL's authenticated exact-fact requeue socket. HAL
accepts only the existing immutable fact and returns it to `PENDING`; neither
service creates replacement evidence or changes the fact payload/digest. CMS
stores requested, failed, or completed platform audit evidence and emits
`hal.fact_requeued` only after HAL accepts the recovery.

`202 Accepted` returns `{"fact_id":"<uuid>","status":"PENDING"}`. HAL
absence returns `404 hal_fact_not_found`; a fact outside reconciliation returns
`409 hal_fact_not_reconciliation_required`; unavailable/not configured HAL
returns `503 hal_unavailable`; other HAL failures return
`502 hal_fact_requeue_failed`.

## 12. Manual CPO Platform Access

Subscription management is documented separately in
`docs/contracts/api/manual-subscriptions.md`. It never changes CPO
administrative access automatically. An expired subscription does block only
new customer charging starts and new customer recharge-order creation; stop,
reconciliation, settlement, and existing payment verification remain allowed.
A platform superadmin grants or removes CPO administrative access directly:

```text
POST /api/v1/platform/cpos/{cpo_id}/activate
POST /api/v1/platform/cpos/{cpo_id}/suspend
```

`ACTIVE` permits eligible CPO administrative authentication and tenant
operations. `SUSPENDED` blocks new tenant access while preserving the CPO and
its historical data. These actions are explicit, audited platform decisions;
there is no payment-driven transition.

### 12.1 Manual Platform Subscriptions

Platform-superadmin subscription routes are:

```text
POST/GET /api/v1/platform/plans
GET /api/v1/platform/plans/{plan_id}
PUT /api/v1/platform/plans/{plan_id}/draft
POST /api/v1/platform/plans/{plan_id}/publish
POST /api/v1/platform/plans/{plan_id}/archive
POST/GET /api/v1/platform/cpos/{cpo_id}/subscription
POST /api/v1/platform/cpos/{cpo_id}/subscription/{renew,change-plan,activate,pause,resume,mark-past-due,expire,cancel}
GET /api/v1/platform/cpos/{cpo_id}/subscription/history
```

They are described completely by
`docs/contracts/api/manual-subscriptions.md` and the canonical OpenAPI schema.
All writes are audited and idempotency-keyed manual commands. The API manages
records only: it has no platform payment provider, invoice/payment flow,
webhook, automatic renewal, scheduled transition, subscription email, or CPO
administrative-authorization effect. The observed lifecycle worker expires
elapsed periods; `renew` reactivates the expired record after manual payment
confirmation and restores new customer starts/recharge-order creation.
Feature keys and entitlement overrides are deliberately not exposed until a
future module catalog defines their server-side enforcement.

### 12.2 Platform governance, security, and mail operations

These routes require a current `PLATFORM` bearer session and are limited to
active platform authority. They never grant tenant business-data access.

Platform administrator governance:

```text
GET  /api/v1/platform/administrators
POST /api/v1/platform/administrators
POST /api/v1/platform/administrators/{user_id}/activate
POST /api/v1/platform/administrators/{user_id}/deactivate
```

The list is cursor-paginated and can include inactive authority records. The
invite request is `{ "email": "...", "full_name": "..." }`. A new global
identity is created with a random password, `MFAEnabled=true`, and
`must_change_password=true`; the temporary password is encrypted into a
`PLATFORM_ADMIN_INVITE` outbox job. Granting an existing active identity does
not replace its password. Activate/deactivate requests require
`{ "reason": "..." }`. Deactivation keeps the identity, revokes its platform
sessions, and refuses to deactivate the last active platform administrator.

Security operations:

```text
GET  /api/v1/platform/security/locked-identities
GET  /api/v1/platform/security/events
POST /api/v1/platform/security/users/{user_id}/unlock
POST /api/v1/platform/security/users/{user_id}/sessions/revoke
```

Unlock requires a reason. Session revocation requires `{ "reason": "...",
"scope": "PLATFORM|CPO|ALL", "cpo_id": "..." }`; `cpo_id` is required only
for `CPO`. It revokes matching administrative sessions and unused refresh
tokens and returns `{ "revoked_sessions": 0, "revoked_refresh_tokens": 0 }`.
Security and audit responses contain safe metadata only.

Mail operations:

```text
GET  /api/v1/platform/mail/jobs
GET  /api/v1/platform/mail/jobs/{job_id}
POST /api/v1/platform/mail/jobs/{job_id}/retry
POST /api/v1/platform/mail/jobs/{job_id}/cancel
GET  /api/v1/platform/mail/metrics
POST /api/v1/platform/mail/reconcile
POST /api/v1/platform/mail/retention
```

Job listing supports status (`PENDING`, `PROCESSING`, `SENT`, `FAILED`, or
`CANCELED`), template, CPO, user, and the standard cursor. Job responses show
recipient and delivery metadata plus `error_present`; they never show the
encrypted payload, decrypted body, temporary password, OTP, or stored error
text. Retry accepts failed/canceled jobs. Cancel accepts pending/failed jobs
and requires a reason. Reconcile requires a reason and requeues processing
jobs locked for more than five minutes. Retention requires `{ "before":
"RFC3339", "reason": "..." }`, where `before` is at least 30 days old, and
deletes only `SENT` or `CANCELED` jobs. Every command is audited and emits the
corresponding committed platform event.

### 12.3 Announcements, notifications, and platform status

Platform communication and status routes are:

```text
GET  /api/v1/platform/announcements
POST /api/v1/platform/announcements
GET  /api/v1/platform/notifications
POST /api/v1/platform/notifications/{notification_id}/read
GET  /api/v1/platform/overview
GET  /api/v1/platform/status
GET  /api/v1/cpo/notifications
POST /api/v1/cpo/notifications/{notification_id}/read
```

Announcement creation accepts `{ "audience": "PLATFORM|CPO", "cpo_id":
"...", "title": "...", "body": "...", "expires_at": "RFC3339" }`.
`cpo_id` is forbidden for `PLATFORM` and required for `CPO`. The transaction
stores the announcement and snapshots currently active platform administrators
or active CPO `ADMIN` members into durable notification rows. Later membership
changes do not rewrite that audience snapshot. Expired announcements are not
returned in recipient notification lists.

Platform recipients use bearer authentication. CPO recipients use their
verified CPO `ADMIN` session plus the current `X-CPO-App-ID` header; the CPO ID
is derived from the authenticated session, not selected by the caller. Both
notification lists are cursor-paginated, support `unread_only`, and expose
read timestamps. Mark-read is recipient-owned and returns `204`; missing or
foreign notifications return `404 notification_not_found`.

Overview returns bounded CPO lifecycle counts, active platform-admin count,
active session count, mail status counts, and worker states. Status returns
service/version, database connectivity, and worker state. The current source
reports version `development` unless a release build supplies a linker value.
Neither endpoint performs unbounded tenant-business aggregation.

### 12.4 CPO charging-session reads

An authenticated CPO member with `charging_sessions.read` can read
materialized charging-session records for the current tenant:

```text
GET /api/v1/cpo/charging-sessions
GET /api/v1/cpo/charging-sessions/{session_id}
```

Both routes require the CPO bearer session and the matching
`X-CPO-App-ID`. The tenant scope is derived from the authenticated session;
there is no client-selectable CPO ID. The list route accepts `limit` (1-200,
default 50), `before` (RFC3339), `before_id` (UUID), `status` (`START_PENDING`,
`ACTIVE`, `STOP_PENDING`, `COMPLETED`, or `FAILED`), `charger_id` (UUID), and
`customer_id` (UUID). Pagination is descending by `(created_at, id)` and the
returned `next_before` plus `next_before_id` form the next exclusive cursor.

Each result contains the CMS session UUID, OCPP transaction ID, tenant-owned
customer/charger/connector UUIDs, start/end timestamps, exact decimal energy
and amount strings, currency, lifecycle status, optional stop reason, and
creation time. These are read-only CMS projections; they do not query the HAL
or issue charger commands. A missing session returns `404
charging_session_not_found`; malformed UUIDs or invalid filters return `400`,
and unauthenticated or unauthorized callers receive the standard `401`/`403`
errors.

Each historical session includes `price_per_unit`, optional tariff `unit`, and
`sgst_percent`, `cgst_percent`, and `igst_percent` from the immutable
session-time tariff/tax snapshots. Current tariff and hub GST associations are
used only as a legacy fallback when a usable snapshot is absent. It also
returns optional `start_criteria` and `requested_limit_value` from the durable
start intent: these describe the customer's `AUTO`, `ENERGY`, `TIME`, or
`MONEY` limit selection and are independent of the tariff billing dimension.
Legacy sessions without a start intent omit those two fields.

### 12.5 CPO charging diagnostic trace

```text
GET /api/v1/cpo/charging-sessions/{session_id}/trace
GET /api/v1/cpo/charging-traces/{trace_id}
GET /api/v1/cpo/charging-traces/{trace_id}/stream
```

Both diagnostic routes require the CPO bearer session, matching
`X-CPO-App-ID`, active membership, and `charging_traces.read`. The default
role bundles grant this to `OWNER`, `ADMIN`, and `OPERATOR`; `VIEWER` is not
granted it, and an explicit membership `DENY` always wins. The current CPO
scope is derived server-side, so a CPO cannot retrieve another tenant's trace.

CMS is the canonical CPO-visible trace store. These reads never query HAL or
merge a runtime HAL response. The static response contains an opaque
`trace_id`, optionally correlated CMS session, HAL transaction, and OCPP
transaction IDs, bounded sanitized evidence, `sources_present`, and a
`replay_cursor`. Those identifiers remain distinct. `sources_present` means
only that CMS currently persists evidence attributed to the listed actor; an
absent HAL/CHARGER source is not a HAL-health assertion. Events are newest
first by the exclusive `(occurred_at,id)` cursor; `limit` is 1–100 (default
50), and both `before_occurred_at` and `before_event_id` must be supplied
together for the next page.

Use the static response's `replay_cursor` as the exclusive `after` query value
(or SSE `Last-Event-ID`) for the stream. It replays immutable `trace_event`
frames by CMS ingestion sequence, while the FE displays events chronologically
by `(occurred_at,id)`. The SSE `id` is that sequence; clients dedupe by event
ID. Event lanes are `APP`, `CMS`, `HAL`, and `CHARGER`; trace rows are
diagnostic evidence only and never determine session, connector, wallet,
billing, settlement, or OCPP state. Event metadata deliberately excludes
idTags, credentials, authorization material, customer contacts, and raw OCPP
frames. `404 charging_trace_not_found` means the diagnostic evidence is
unavailable or retained out; it does not mean the underlying charging session
is invalid.

### 12.6 CPO charger transaction reads

`GET /api/v1/cpo/charger-transactions` returns a cursor-paginated transaction
projection for the authenticated CPO member with `charging_sessions.read`. It accepts `limit` (1–200,
default 50), `before` (RFC3339), `before_id` (UUID), and optional same-CPO
`charger_id` or `customer_id` UUID filters. Results are ordered newest first
by `(created_at, id)`; the returned `next_before` and `next_before_id` form the
next exclusive cursor.

Each transaction includes the session UUID, financial status, decimal-string
`billed_amount` and `usage_kwh`, tariff unit price, charger ID, elapsed
duration when complete, hub, CPO owner, host contact projection, customer
contact projection, creation timestamp, and optional stop reason. The read is
tenant-scoped and does not contact the HAL or issue charger commands. Financial
status values are `PENDING`, `COMPLETED`, `FAILED`, `REVERSED`, or `REFUNDED`.
Malformed filters return `400`; unauthenticated or unauthorized callers receive
the standard `401`/`403` responses.

## 13. Client State Machine

Recommended frontend sequence:

```text
login request
  -> store challenge ID only
  -> collect emailed OTP
  -> verify
  -> store access + current refresh token securely
  -> call /auth/me
  -> if must_change_password: force password-change screen
  -> otherwise enter platform or CPO application
  -> for CPO business calls attach current X-CPO-App-ID
  -> refresh before/after access expiry
  -> atomically replace refresh token
```

On `401 unauthorized`, clear access state and attempt refresh only if the client
still owns the current refresh token. On `401 invalid_refresh_token`, clear the
entire local session and require login. On `cpo_app_id_mismatch`, refresh or call
`/auth/me` to recover the current ID; never let a user type a CPO ID to change
scope.

## 14. Explicitly Unimplemented

The contract does not provide:

- customer profile/email editing;
- CPO staff invitation after the first administrator;
- custom roles or permission APIs;
- standalone connector creation/deletion; GST or tariff deletion;
  CPO-side wallet mutation, payment, or reporting APIs;
- any tenant-side CPO profile route;
- payment execution or Razorpay webhook verification;
- CMS/HAL commands or callbacks;
- platform invoices or platform payment management;
- OpenAPI-generated SDKs.

Database tables for several future domains do not imply callable APIs.
