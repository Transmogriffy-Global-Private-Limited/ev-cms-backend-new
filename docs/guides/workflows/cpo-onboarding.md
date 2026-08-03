# CPO Onboarding and Recovery Workflow

## Outcome

A platform superadmin can take a CPO from creation to usable tenant access and
recover the CPO's administrator access without subscriptions, payment state, or
undocumented database intervention.

## Preconditions

- The superadmin bootstrap and platform OTP login work.
- `MAIL_ENABLED=true`; the durable mail worker is fresh and healthy.
- The slug and optional GSTIN are unique.
- The first administrator email is correct. Generated passwords are never
  returned by an API.

## Primary Flow

```text
platform login + OTP
→ create PENDING CPO and primary admin
→ inspect safe onboarding-mail status
→ activate with reason
→ administrator CPO login + OTP
→ required password change, if new identity
→ administrator logs in again
→ optional live app-ID replacement
→ CPO tenant APIs use the current X-CPO-App-ID
```

1. Start platform login with `POST /api/v1/auth/login`, then verify the emailed
   OTP through `POST /api/v1/auth/2fa/verify`.
2. Call `POST /api/v1/platform/cpos`. The transaction creates the `PENDING`
   CPO, generated dummy app ID, exactly one primary `ADMIN` membership, audit
   evidence, durable platform event, and encrypted mail job. For a new global
   identity, that welcome job is rejected if its generated temporary password
   is missing; an existing identity receives no new password.
3. Call
   `GET /api/v1/platform/cpos/{cpo_id}/primary-admin`. Display the safe outbox
   status; never attempt to display a password or encrypted payload.
4. Call `POST /api/v1/platform/cpos/{cpo_id}/activate` with an operator reason.
   A live app ID and commercial record are not prerequisites.
5. The administrator starts a `CPO` login using CPO ID, password, and email OTP.
6. If `must_change_password=true`, keep tenant operations blocked and call
   `POST /api/v1/auth/password/change`. The password change revokes all of that
   identity's sessions, so discard tokens and sign in again.
7. Bootstrap the tenant UI from `GET /api/v1/auth/me`. Tenant business requests
   send the returned current app ID in `X-CPO-App-ID`.
8. When the CPO's application goes live, the platform user may replace the
   dummy ID through `PUT /api/v1/platform/cpos/{cpo_id}/app-id`. Tenant clients
   recover the new value through login, refresh, or `/auth/me`.

## Existing Identity

If the submitted administrator email already belongs to an active global
identity, creation does not overwrite its name, password, verification state,
or existing memberships. The new CPO membership is added and a credential-free
assignment email is queued. An inactive global identity is rejected.

## Recovery Decision Tree

```text
Administrator cannot access CPO
├─ still the correct active primary
│  ├─ forgot password → recovery start exists; FE completion is currently blocked
│  └─ lost CPO/app details → resend onboarding details
├─ membership was revoked but same person remains responsible
│  └─ assign the same email as primary to restore membership
├─ responsibility changed
│  └─ replace primary; previous primary CPO sessions are revoked
└─ suspected CPO-wide session compromise
   └─ revoke all CPO administrative sessions with a reason
```

Resend uses
`POST /api/v1/platform/cpos/{cpo_id}/primary-admin/resend-onboarding`.
It queues only current CPO/app details and password-recovery guidance.

The password-recovery email now contains both the opaque recovery ID and OTP
required by reset. Onboarding resend remains credential-free: it never reads or
rotates a global password, and directs an administrator who lacks credentials
to request a fresh recovery email.

Replacement uses
`PUT /api/v1/platform/cpos/{cpo_id}/primary-admin`. It is transactional and
serialized. The previous primary membership and CPO sessions are revoked, the
new or existing eligible identity becomes the sole primary administrator, and
the appropriate encrypted onboarding mail is queued.

CPO-wide session recovery uses
`POST /api/v1/platform/cpos/{cpo_id}/administrative-sessions/revoke`.
It affects only CPO-scope sessions for that tenant. Customer and platform
sessions remain valid.

## Failure, Retry, and Recovery

- Mail-disabled creation or administrator replacement fails before durable
  state changes.
- Unique constraints reject duplicate slug, GSTIN, app ID, and primary-admin
  state.
- Repeating activation or suspension does not duplicate lifecycle audit/events.
  Repeated suspension still revokes sessions created afterward.
- Repeating session revocation is safe and returns zero counts when nothing
  remains.
- Mail delivery retries from the encrypted durable outbox after process restart.
- Realtime is advisory. Reconnect with the event cursor and reload CPO REST
  state after cursor expiry.
- Suspension blocks new CPO-staff and customer login, preserves tenant data,
  and revokes existing tenant sessions.

## Verification and Evidence

Safe support evidence includes CPO ID, lifecycle state/reason/actor/time,
primary-admin user ID and membership status, outbox job ID/status/attempts,
revoked-session counts, audit action, and platform-event ID.

Never copy temporary passwords, OTPs, tokens, encrypted payloads, SMTP
credentials, or CPO integration secrets into tickets or logs.

Exact bodies, responses, validation rules, and errors are in
`../../contracts/api/administrative-http-api.md`; the frontend-specific model is
in `../../CPO_ADMINISTRATION.md`.
