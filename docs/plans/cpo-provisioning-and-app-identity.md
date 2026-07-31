# CPO Provisioning and Application Identity

Status: Verified

## Objective

Provision CPO tenants through explicit manual platform lifecycle control.
Give each CPO a stable application identifier for
tenant request routing, beginning with a dummy onboarding ID and changing to a
superadmin-assigned live ID when its application goes live.

Creation also establishes the first CPO administrator so the tenant has a
usable human owner of onboarding work.

## Lifecycle and Identity

The two state machines are independent:

```text
CPO lifecycle: PENDING -> ACTIVE <-> SUSPENDED
App identity:  DUMMY -> LIVE -> LIVE (rotation)
```

- Creation is `PENDING` with a unique server-generated dummy app ID.
- Activation permits CPO login and tenant operations with the current dummy or
  live app ID.
- Suspension blocks new CPO-staff and customer operations regardless of app-ID
  mode.
- Assigning a live app ID does not imply commercial or payment state.
- App-ID rotation invalidates the old header value immediately.

There is deliberately no active subscription table, foreign key, entitlement
check, or feature matrix. The retired migration-seven/eight prototype is
archived outside the runtime schema by migration nine.

## First CPO Administrator

Creation requires the first administrator's email and full name.

- If the email is new, the backend generates a high-entropy temporary password,
  stores only its Argon2id hash, creates an active identity with
  `must_change_password=true`, grants the CPO `ADMIN` membership, and writes an
  encrypted welcome-mail job in the same transaction.
- If the global identity already exists, the backend never replaces its
  password. It grants the new CPO `ADMIN` membership and writes an encrypted
  assignment-mail job.
- The initial password has no hard expiry. This avoids locking an administrator
  out before a delayed onboarding.
- Every successful login while `must_change_password=true` writes an encrypted
  reminder-mail job.
- Authentication, password-change/recovery, logout, and session-control APIs
  remain usable. Tenant business APIs return `password_change_required` until
  password change or reset clears the flag.
- Password change/reset clears the flag and revokes every existing session.

Temporary password plaintext exists only in application memory during creation,
the encrypted mail payload, and mail-worker memory during delivery. It is never
returned by the API, logged, audited, or stored in plaintext.

The welcome or assignment email includes the CPO ID and current app ID. The
new-identity welcome additionally includes the temporary password.

One `OWNER` or `ADMIN` membership is durably designated primary for each
provisioned CPO. Platform APIs expose only safe identity, membership, and
onboarding-delivery metadata. A platform actor may replace or restore the
primary with a reason; replacement revokes the previous primary's sessions for
that CPO without changing an existing replacement identity's password.
Onboarding resend is credential-free and directs the recipient to normal
password recovery.

## Request Boundary

Header:

```http
X-CPO-App-ID: <current-cpo-app-id>
```

For a CPO-scoped business request:

1. Decrypt and verify the access token.
2. Revalidate the durable session, CPO, membership, and role.
3. Read the current app ID for that same authenticated CPO.
4. Require an exact constant-time header match.
5. Continue using the principal's CPO ID; never derive tenant context from the
   header.

The app ID is distributed application metadata, not a password or API secret.
Possession of it grants no authority.

## Catch-22 Exemptions

The header is not required for:

- `/health/*`
- `/api/v1/auth/*`, including login, OTP, refresh, `me`, logout, sessions, and
  password recovery/change
- `/api/v1/platform/*`
- future HAL and provider callbacks that will use independent service or
  webhook authentication

CPO OTP verification, refresh, and `me` return the current app ID. A client can
therefore learn the dummy ID after first login and recover a rotated live ID
without already knowing it.

## Platform API

- `POST /api/v1/platform/cpos`
- `GET /api/v1/platform/cpos`
- `GET /api/v1/platform/cpos/:cpo_id`
- `PUT /api/v1/platform/cpos/:cpo_id/profile`
- `POST /api/v1/platform/cpos/:cpo_id/activate`
- `POST /api/v1/platform/cpos/:cpo_id/suspend`
- `PUT /api/v1/platform/cpos/:cpo_id/app-id`
- `GET|PUT /api/v1/platform/cpos/:cpo_id/primary-admin`
- `POST /api/v1/platform/cpos/:cpo_id/primary-admin/resend-onboarding`
- `POST /api/v1/platform/cpos/:cpo_id/administrative-sessions/revoke`

Creation accepts the existing CPO business-profile fields but not status,
commercial state, or app ID. It also accepts the first administrator's email
and name. The server owns lifecycle and app-ID values.

Live app IDs are 16 to 100 lowercase URL/header-safe characters. Dummy IDs use
the reserved `cpo_dummy_` prefix; superadmin-supplied live IDs cannot use it.

## Failure and Recovery

- Unique indexes prevent duplicate slug, GSTIN, and app ID values.
- A per-email PostgreSQL advisory transaction lock serializes concurrent
  new-versus-existing first-admin identity decisions.
- Creation and audit insertion share one transaction.
- First-admin identity/membership and onboarding mail share that transaction.
- A mail-disabled deployment rejects creation instead of exposing a temporary
  password through another channel.
- Existing identities are reused without password mutation.
- Lifecycle transitions require a reason and are idempotent for the requested
  target state; repeated suspension still revokes later CPO sessions.
- Primary-admin replacement is serialized per CPO and normalized email, with a
  partial unique index enforcing one primary membership.
- A duplicate live app ID returns conflict without changing the CPO.
- Rotation preserves sessions but tenant APIs reject the old app ID.
- Authentication endpoints remain available after rotation and suspension for
  safe recovery/logout; new CPO login still requires an active CPO.

## Verification

- Migration contains matching additive up/down behavior.
- Unit tests cover generation, validation, middleware mismatch, and platform
  authorization.
- PostgreSQL integration tests cover creation without a commercial prerequisite, unique
  dummy ID, concurrent and sequential new/existing first-admin behavior,
  encrypted temporary-password delivery, repeated login reminders,
  password-change enforcement, activation, dummy-header access, live-ID
  rotation, old-ID rejection, and suspension.
- Full Go tests, vet, diff check, and residue scan pass.
