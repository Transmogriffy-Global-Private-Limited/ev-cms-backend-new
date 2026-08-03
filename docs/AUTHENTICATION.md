# Authentication and Credential API

For a complete endpoint-by-endpoint contract, use
`docs/contracts/api/administrative-http-api.md`. The validated machine-readable
contract is `docs/contracts/openapi/openapi.yaml`; while the service runs it is
available through Swagger UI at `/docs/` and directly at `/openapi.yaml`.
SuperAdmin frontend implementers should start with
`docs/SUPERADMIN_FRONTEND_HANDOFF.md`.

## Boundary

This API authenticates administrative identities in one of two explicit
scopes:

- `PLATFORM`: requires a `platform_admins` record and has no CPO context.
- `CPO`: currently requires an active `ADMIN` membership in an active CPO. The
  verified session carries the CPO ID and ADMIN role. Other persisted fixed-role
  values are dormant until a future staff-management contract activates them.

Every administrative login requires password plus email OTP. A client cannot
choose tenant authority through a header: protected code reads user ID, session
ID, CPO ID, and role from the validated principal.

Customer/app-user authentication is a separate `CUSTOMER` session plane under
`/api/v1/app/auth`. It includes email-verified signup, password-plus-mail-OTP
login, encrypted access tokens, rotating refresh tokens, `me`, customer-scoped
session management, logout, and password recovery/change. Customer tokens
cannot authorize the administrative routes documented below. Staff
invitations, social login, SMS, TOTP, and passkeys are not implemented.

## Required Configuration

The application loads the ignored `.env` file and then process environment.
Use `.env.example` as the canonical list.

Required for startup:

- `DATABASE_URL`
- `SUPERADMIN_EMAIL`
- `SUPERADMIN_PASSWORD` (10 to 128 characters)
- `JWT_SIGNING_KEY_B64` (at least 32 decoded bytes)
- `JWT_ENCRYPTION_KEY_B64` (exactly 32 decoded bytes)
- `OTP_HMAC_KEY_B64` (at least 32 decoded bytes)
- `MAIL_OUTBOX_ENCRYPTION_KEY_B64` (exactly 32 decoded bytes)
- `CREDENTIAL_ENCRYPTION_KEY_B64` (exactly 32 decoded bytes)

When `MAIL_ENABLED=true`, also configure:

- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_FROM_ADDRESS`
- `SMTP_USERNAME` and `SMTP_PASSWORD` together, or neither
- exactly one encrypted transport:
  - `SMTP_USE_SSL=true` and `SMTP_USE_TLS=false` for implicit TLS from
    connection start;
  - `SMTP_USE_TLS=true` and `SMTP_USE_SSL=false` for mandatory STARTTLS.

The deployed Hostinger contract uses `smtp.hostinger.com:465`, implicit TLS,
and the `team@transev.in` mailbox. Its password remains only in the ignored
`.env` file or process environment. Plaintext SMTP and ambiguous transport
configuration are rejected at startup.

Use independent random key values. Never reuse the JWT signing key as an
encryption or OTP key.

## Initial Superadmin

Startup is safe to repeat:

1. If the configured email does not exist, it creates an active, verified user
   with an Argon2id password hash and grants platform authority.
2. If the user exists without platform authority, it grants that authority.
3. If the platform admin already exists, startup succeeds without changing the
   password.

Bootstrap is serialized with a PostgreSQL advisory transaction lock so two
application instances cannot create duplicate identities. The password is
never logged.

## Common HTTP Contract

Base path: `/api/v1`

Protected endpoints require:

```http
Authorization: Bearer <encrypted-access-token>
```

Authentication and credential responses set `Cache-Control: no-store`.
Request bodies are size bounded and reject unknown JSON fields.

Error shape:

```json
{
  "error": {
    "code": "invalid_credentials",
    "message": "The supplied credentials or scope are invalid."
  }
}
```

The API deliberately uses generic login, challenge, and password-recovery
responses to avoid revealing whether an email or authority exists.

## Login

### Start platform login

`POST /api/v1/auth/login`

```json
{
  "email": "admin@example.com",
  "password": "<password>",
  "scope": "PLATFORM"
}
```

### Start CPO login

```json
{
  "email": "cpo-admin@example.com",
  "password": "<password>",
  "scope": "CPO",
  "cpo_id": "c821a013-5041-42f7-80c8-aa153cf9d455"
}
```

Successful status: `202 Accepted`

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
  "expires_at": "2026-07-23T12:10:00Z",
  "resend_available_at": "2026-07-23T12:01:00Z"
}
```

The OTP is mailed; it is never returned by the API or stored in plaintext.

### Verify login OTP

`POST /api/v1/auth/2fa/verify`

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497",
  "code": "123456"
}
```

Successful status: `200 OK`

```json
{
  "access_token": "<signed-and-encrypted-JWT>",
  "access_token_expires_at": "2026-07-23T12:15:00Z",
  "refresh_token": "<opaque-one-time-token>",
  "session_expires_at": "2026-08-22T12:00:00Z",
  "token_type": "Bearer",
  "cpo_app_id": "cpo_dummy_735f36a898b84ce68a350db38c90bf9b",
  "cpo_app_id_mode": "DUMMY",
  "must_change_password": true
}
```

The access token is a compact, signed-then-encrypted JWT. The refresh token is
opaque; only its SHA-256 hash is stored. CPO-scoped responses expose the
current application ID and its `DUMMY` or `LIVE` mode. Platform responses omit
those two fields. `must_change_password` is present in both scopes.

### Resend login OTP

`POST /api/v1/auth/2fa/resend`

```json
{
  "challenge_id": "5cef4c95-a1da-448e-bd7c-19d570cd4497"
}
```

Successful status: `202 Accepted`. Resend is allowed only after the reported
cooldown and invalidates the previous challenge.

## Token Refresh

`POST /api/v1/auth/refresh`

```json
{
  "refresh_token": "<current-refresh-token>"
}
```

Successful status: `200 OK` with the same token response shape. The submitted
refresh token becomes unusable and a new one is returned. Reuse of a consumed
token revokes the entire session.

Every protected request decrypts and verifies the access token and then checks
the current user, session, platform/CPO authority, membership, and CPO status in
PostgreSQL. Logout and suspension therefore do not wait for token expiry.

## Identity and Session Management

### Current principal

`GET /api/v1/auth/me`

Returns global identity metadata plus the current scope, CPO ID, and role.
CPO-scoped responses also return the current CPO app ID and mode. The nested
user includes `must_change_password`. Password hashes and token material are
never returned.

### List active sessions

`GET /api/v1/auth/sessions`

Returns active sessions for the current identity, including scope, safe client
metadata, expiry, and `is_current`.

### Revoke a session

`DELETE /api/v1/auth/sessions/{session_id}`

The session must belong to the current identity. Successful status:
`204 No Content`.

### Logout current session

`POST /api/v1/auth/logout`

Successful status: `204 No Content`.

### Logout every session

`POST /api/v1/auth/logout-all`

Successful status: `204 No Content`.

## Password Management

### Forgot password

`POST /api/v1/auth/password/forgot`

```json
{
  "email": "admin@example.com"
}
```

Always returns the same `202 Accepted` response for eligible and unknown email
addresses. An eligible identity receives an encrypted-outbox OTP.

The response intentionally omits all challenge material. For an eligible
active identity, the encrypted recovery email supplies the opaque recovery ID
(`challenge_id`), six-digit code, and shared expiry. This preserves the same
generic API response for eligible and unknown addresses while giving the
recipient every input required by reset.

OTP producers call the canonical mail enqueue operation with the complete
payload, including `challenge_id`, for validation and encrypted outbox storage;
there is no field-subsetting OTP mapper. The active development VPS revision
`d27e599` predates this source-tree correction as of 2026-08-03 and can return
`500 internal_error` for an eligible recovery request until a corrected
revision is explicitly deployed.

### Reset password

`POST /api/v1/auth/password/reset`

```json
{
  "challenge_id": "988127b5-3954-4e46-9876-f90eeec5de26",
  "code": "123456",
  "new_password": "<new-password>"
}
```

Successful reset consumes the challenge and revokes every existing session and
refresh token.

Old recovery emails created before recovery-ID delivery was implemented cannot
complete reset; the user must request a new email. Successful reset consumes
the emailed ID/code pair, and no internal database lookup is required by the
client.

### Change password

`POST /api/v1/auth/password/change`

```json
{
  "current_password": "<current-password>",
  "new_password": "<new-password>"
}
```

This endpoint is authenticated. Successful change also revokes every session,
including the current one; the user must sign in again.

The initial administrator created with a CPO receives an autogenerated
temporary password by encrypted outbox email. That password has no hard
expiry. Until it is changed or reset, every successful login queues a reminder
email and tenant business APIs return `password_change_required`.

## Authorization Helpers

Later handlers use:

- `auth.CurrentPrincipal`
- `auth.CurrentUserID`
- `auth.CurrentCPOID`
- `auth.CurrentCPOAppID`
- `auth.RequirePlatform`
- `auth.RequireCPORoles`
- `auth.RequireCPOAppID`

These helpers consume only the server-validated Gin context. They do not trust
request tenant IDs. Tenant business APIs require `X-CPO-App-ID` to match the
current app ID read for the authenticated CPO. The header is routing metadata,
not authentication, and never chooses the tenant.

## CPO Integration Credentials

Only a CPO `ADMIN` session may use these endpoints. `OWNER`, `OPERATOR`, and
`VIEWER` remain dormant schema values for a future staff-management feature and
cannot currently establish callable CPO authority. Platform
superadmins deliberately have no route to tenant secret plaintext.

Every request in this section also requires:

```http
X-CPO-App-ID: <current-dummy-or-live-app-id>
```

Supported provider: `RAZORPAY`.

### Configure or rotate

`PUT /api/v1/cpo/integrations/RAZORPAY`

```json
{
  "key_id": "<razorpay-key-id>",
  "key_secret": "<razorpay-key-secret>",
  "webhook_secret": "<optional-webhook-secret>"
}
```

The full JSON credential object is encrypted with AES-256-GCM and authenticated
against the verified CPO ID plus provider. A write response returns metadata
only:

```json
{
  "provider": "RAZORPAY",
  "display_hint": "****5678",
  "is_active": true,
  "configured_at": "2026-07-23T12:00:00Z",
  "updated_at": "2026-07-23T12:00:00Z"
}
```

### List metadata

`GET /api/v1/cpo/integrations`

### Get provider metadata

`GET /api/v1/cpo/integrations/RAZORPAY`

### Remove credentials

`DELETE /api/v1/cpo/integrations/RAZORPAY`

Successful status: `204 No Content`.

There is no user-facing secret read endpoint. Future payment orchestration uses
the internal, tenant-scoped `ResolveRazorpay` method.

## Durable Mail Worker

OTP and onboarding-message creation share a transaction with the state change
that requires the message. Every mail payload is encrypted before insertion.
OTP producers directly enqueue the complete structured payload so
template-specific fields such as recovery `challenge_id` cannot be dropped by
an intermediate mapper.
The worker:

1. claims one eligible job with `FOR UPDATE SKIP LOCKED`;
2. reclaims processing jobs abandoned for five minutes;
3. decrypts only in worker memory;
4. sends over the configured mandatory encrypted SMTP transport;
5. records success or bounded exponential retry;
6. stops after eight failed attempts.

OTP values are not logged. SMTP delivery errors are bounded before storage.

## Security and Operational Notes

- Password login locks an identity temporarily after repeated failures.
- Login, OTP, recovery, and refresh operations also use durable hashed
  source-address rate-limit keys. Expired rate-limit rows are pruned
  opportunistically.
- OTPs expire, have bounded attempts, and are bound to challenge ID and purpose.
- Secret encryption keys have explicit IDs. Before removing an old key, records
  using it must be re-encrypted; automatic rotation is deferred.
- Audit logs record privileged auth and integration mutations but never
  passwords, OTPs, refresh tokens, or provider credentials.
- Database warning/error logging uses parameter placeholders and suppresses
  ordinary record-not-found queries so submitted credentials and identity
  values are not copied into SQL logs.
- HTTP completion logs contain only the matched route template, status,
  latency, stable handled error code, and safe opaque auth identifiers. They
  never contain auth/recovery bodies, raw paths, queries, header values, email,
  user agents, API messages, tokens, passwords, or OTPs. Use the returned
  `X-Request-ID` for correlation.
- Under `LOG_LEVEL=DEBUG`, authentication failures add only the stable API
  code, owning `auth` component, status, and Go error type. Submitted identity,
  credential, token, error message, and database/provider values stay excluded.
