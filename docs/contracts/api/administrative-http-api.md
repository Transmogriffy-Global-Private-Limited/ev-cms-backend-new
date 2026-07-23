# Administrative HTTP API: Complete Developer Contract

This is the human-readable contract for every currently implemented HTTP
endpoint. It is intended to be sufficient for frontend, QA, mobile, and backend
integration without reading Go source.

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

## 4. Authentication Workflow

### 4.1 `POST /api/v1/auth/login`

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

### 4.2 `POST /api/v1/auth/2fa/verify`

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

### 4.3 `POST /api/v1/auth/2fa/resend`

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

### 4.4 `POST /api/v1/auth/refresh`

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

## 5. Password Operations

### 5.1 `POST /api/v1/auth/password/forgot`

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
`PASSWORD_RESET_OTP` job.

Operational errors: `400 invalid_request`, `429 rate_limited`,
`503 mail_unavailable`, or `500 internal_error`.

### 5.2 `POST /api/v1/auth/password/reset`

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

Errors:

- `400 invalid_request`;
- `400 invalid_password` with the exact length-policy explanation;
- `401 invalid_challenge`;
- `429 rate_limited`;
- `500 internal_error`.

### 5.3 `POST /api/v1/auth/password/change`

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

## 6. Identity and Sessions

### 6.1 `GET /api/v1/auth/me`

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

### 6.2 `GET /api/v1/auth/sessions`

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

### 6.3 `DELETE /api/v1/auth/sessions/{session_id}`

Authentication: bearer session.

`204 No Content` revokes the selected owned session and its unused refresh
tokens. It may revoke the current session.

Errors:

- `400 invalid_request`: malformed UUID;
- `401 unauthorized`;
- `404 session_not_found`: missing or owned by another identity;
- `500 internal_error`.

### 6.4 `POST /api/v1/auth/logout`

Authentication: bearer session.

`204 No Content` revokes the current session and its unused refresh tokens.
The current access token becomes unusable on the next request.

### 6.5 `POST /api/v1/auth/logout-all`

Authentication: bearer session.

`204 No Content` revokes every platform and CPO session for the current global
identity, plus all unused refresh tokens.

## 7. Platform CPO Control Plane

All endpoints in this section require a bearer token whose durable session has
`PLATFORM` scope. `X-CPO-App-ID` is neither required nor accepted as tenant
authority.

### 7.1 `POST /api/v1/platform/cpos`

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
- optional `gstin`: uppercased, exactly 15 uppercase letters/digits;
- address/city/state/pincode maxima: 5000/100/100/10;
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
- `409 cpo_conflict` for unique slug/GSTIN/app-ID/membership collisions;
- `409 admin_identity_inactive`;
- `503 mail_unavailable`;
- `500 internal_error`.

### 7.2 `GET /api/v1/platform/cpos`

Returns:

```json
{"cpos":[/* CPO objects */]}
```

The collection contains at most the 100 newest CPOs. No query filters,
pagination cursor, subscription, or entitlement data exists.

### 7.3 `GET /api/v1/platform/cpos/{cpo_id}`

Returns one CPO object.

Errors: `400 invalid_cpo_id`, `401 unauthorized`, `403 forbidden`,
`404 cpo_not_found`, or `500 internal_error`.

### 7.4 `POST /api/v1/platform/cpos/{cpo_id}/activate`

No body.

Sets status to `ACTIVE` and returns the CPO object. Calling it on an already
active CPO is idempotent. It does not create/check a subscription and does not
require a live app ID.

### 7.5 `POST /api/v1/platform/cpos/{cpo_id}/suspend`

No body.

Sets status to `SUSPENDED`, revokes every active CPO session and unused refresh
token for that tenant, records audit state, and returns the CPO. Repeated calls
are lifecycle-idempotent. Platform sessions are unaffected.

### 7.6 `PUT /api/v1/platform/cpos/{cpo_id}/app-id`

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
`404 cpo_not_found`, `409 cpo_conflict`, or `500 internal_error`.

## 8. CPO Integration Credentials

All endpoints require:

- bearer CPO session;
- role `OWNER` or `ADMIN`;
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

### 8.1 `GET /api/v1/cpo/integrations`

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

### 8.2 `GET /api/v1/cpo/integrations/{provider}`

Returns the same metadata object.

Additional errors:

- `400 unsupported_integration_provider`;
- `404 integration_not_found`.

### 8.3 `PUT /api/v1/cpo/integrations/{provider}`

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

### 8.4 `DELETE /api/v1/cpo/integrations/{provider}`

`204 No Content` deletes the encrypted row and records an audit event.

Additional errors: `400 unsupported_integration_provider` and
`404 integration_not_found`.

## 9. Client State Machine

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

## 10. Explicitly Unimplemented

The contract does not provide:

- public customer signup/login;
- CPO staff invitation after the first administrator;
- custom roles or permission APIs;
- hub, charger, connector, tariff, wallet, charging, payment, or reporting
  APIs;
- payment execution or Razorpay webhook verification;
- CMS/HAL commands or callbacks;
- subscriptions or entitlement checks;
- OpenAPI-generated SDKs.

Database tables for several future domains do not imply callable APIs.
