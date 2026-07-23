# Authentication and Credential Boundary

Status: Verified

## Objective

Implement the smallest complete credential boundary that can safely authenticate
platform superadmins and CPO staff, support every later protected API, recover
accounts, manage durable sessions, deliver administrative OTP email, and store
tenant payment-provider credentials without exposing their plaintext.

## Security Boundary

- A login identity is global.
- A session has exactly one trusted scope: `PLATFORM` or `CPO`.
- `PLATFORM` scope requires an active user and a `platform_admins` record.
- `CPO` scope requires an active user, active CPO, and active CPO membership.
- The CPO identifier and fixed membership role come from the verified session.
  A client-supplied tenant header is never authoritative.
- Platform authority does not grant tenant secret access.

## Credential and Token Design

- Passwords use Argon2id with a per-password random salt and the
  memory-constrained RFC 9106 profile.
- Administrative login is two-step: password, then a single-use email OTP.
- OTPs are generated cryptographically, stored only as keyed hashes, expire
  quickly, have bounded attempts, and are never logged.
- Access tokens are nested signed-then-encrypted JWTs. The inner token is signed
  with a fixed algorithm; the outer token uses authenticated encryption. Every
  request validates type, algorithms, issuer, audience, time, subject, session,
  and scope.
- Refresh tokens are opaque random values. Only their hashes are stored. Every
  successful refresh rotates the token; reuse revokes the session.
- Protected requests also verify current durable session, identity, and scope
  state so logout, password reset, or suspension takes effect promptly.

## Bootstrap

Startup reads the initial superadmin email, password, and optional display name
from process environment or the ignored `.env` file.

- If the configured platform administrator already exists, startup succeeds and
  does not change the password.
- If the user exists without platform authority, startup grants the platform
  administrator record transactionally.
- Otherwise startup creates an active, email-verified user and platform
  administrator transactionally.
- Secret values are never committed or logged.

## Mail Delivery

OTP-producing commands write an encrypted mail payload to a PostgreSQL outbox in
the same transaction as the challenge. A worker claims jobs using row locking,
delivers through exactly one explicitly configured encrypted SMTP transport,
records success, and retries temporary failure with bounded backoff. The
current Hostinger configuration uses implicit TLS on port 465. Mandatory
STARTTLS remains supported as a separate mode; plaintext and ambiguous
transport configuration are rejected. No Redis or separate broker is needed
for this workload.

If mail delivery is disabled or unavailable, commands that require a new OTP
fail safely instead of logging or returning the OTP.

## API Surface

Public authentication:

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/2fa/verify`
- `POST /api/v1/auth/2fa/resend`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/password/forgot`
- `POST /api/v1/auth/password/reset`

Authenticated identity and sessions:

- `GET /api/v1/auth/me`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/logout-all`
- `GET /api/v1/auth/sessions`
- `DELETE /api/v1/auth/sessions/:session_id`
- `POST /api/v1/auth/password/change`

CPO integration credentials:

- `GET /api/v1/cpo/integrations`
- `GET /api/v1/cpo/integrations/:provider`
- `PUT /api/v1/cpo/integrations/:provider`
- `DELETE /api/v1/cpo/integrations/:provider`

The first supported provider is `RAZORPAY`. Its key ID, key secret, and optional
webhook secret are encrypted together with CPO/provider-bound authenticated
encryption. Read APIs return only provider, status, key identifier hint, and
timestamps. Secret plaintext has no read API.

## Failure and Recovery

- Repeated bootstrap is idempotent.
- Login and recovery responses do not reveal whether an identity exists.
- Repeated OTP verification cannot consume a challenge twice.
- OTP resend invalidates the previous challenge.
- Refresh rotation is transactional; observed reuse revokes the session.
- Password change or reset revokes all existing sessions.
- Mail jobs survive restart and abandoned worker locks are reclaimed.
- Credential replacement is atomic and audited without secret fields.

## Implementation Slices

1. Completed: durable auth, session, challenge, mail-outbox, rate-limit, and
   encrypted tenant-credential schema.
2. Completed: configuration validation, cryptography, bootstrap, and mail
   worker.
3. Completed: authentication, recovery, session, middleware, and helper APIs.
4. Completed: CPO integration-credential service and APIs.
5. Completed: focused, PostgreSQL, route-level, and full verification plus
   project-memory reconciliation.

## Acceptance Criteria

- All criteria in the development-plan feature entry pass.
- No secret or OTP appears in API output, database plaintext, application logs,
  examples, or committed files.
- All mutations are transactionally scoped and tenant authorization is derived
  from the verified principal.
- Focused and repository-wide verification pass.

## Deferred Work

- Public customer authentication and signup flows
- Email-address change workflow
- Authenticator-app TOTP, SMS, passkeys, and social identity
- Automatic cryptographic-key rotation
- General-purpose tenant secret storage
- Payment-provider calls and webhook verification

## Verification Result

- Password, random-value, secret-box, encrypted-token, OTP-binding,
  configuration, retry, authorization, route-registration, and model tests
  passed.
- PostgreSQL 17 migration down, up, and idempotent up passed.
- PostgreSQL integration tests passed for idempotent bootstrap without password
  overwrite, platform and CPO email-OTP login, encrypted token validation,
  refresh rotation and reuse revocation, password recovery, durable mail-worker
  claiming/decryption/completion, and encrypted Razorpay credential isolation.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- Hostinger implicit-TLS client construction and configuration validation
  passed. Delivery through the real mailbox remains operationally unverified;
  no credential was placed in the repository or test output.
