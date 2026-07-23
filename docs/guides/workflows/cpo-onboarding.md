# CPO Onboarding Workflow

## Outcome

A platform superadmin creates a CPO without a subscription, assigns its first
administrator, activates it when ready, and later replaces its dummy app ID
with the real production identifier.

## Preconditions

- The initial superadmin bootstrap has completed.
- `MAIL_ENABLED=true` and the encrypted outbox worker is operational.
- The superadmin can receive login OTP email.
- The intended CPO slug and optional GSTIN are unique.
- The first administrator email is correct. The API never returns a generated
  temporary password for manual correction.

## Workflow

1. The superadmin signs in using platform password plus email OTP.
2. `POST /api/v1/platform/cpos` creates the pending CPO, unique dummy app ID,
   first `ADMIN` membership, audit record, and encrypted mail job in one
   transaction.
3. For a new email, the CMS generates a temporary password, stores only its
   Argon2id hash, and places the plaintext only inside the encrypted welcome
   mail payload. For an existing active identity, it preserves the password and
   sends an assignment notice.
4. The superadmin calls the activation endpoint. Activation does not require a
   subscription or live app ID.
5. The CPO administrator signs in with CPO scope, CPO ID, password, and the
   emailed OTP.
6. A new identity receives `must_change_password: true`. Authentication and
   password-management routes remain available, while tenant business routes
   reject access until the password changes. Every successful login queues a
   reminder; the temporary password has no hard timeout.
7. Password change revokes all sessions. The administrator signs in again with
   the new password.
8. When the customer app goes live, the superadmin calls
   `PUT /api/v1/platform/cpos/{cpo_id}/app-id`. The new ID applies immediately.
9. CPO clients send the current ID in `X-CPO-App-ID` on tenant business routes.

## Exact API Sequence

1. Call `POST /api/v1/auth/login` with `scope: PLATFORM`.
2. Collect the emailed OTP and call `POST /api/v1/auth/2fa/verify`.
3. Put the returned access token in `Authorization: Bearer ...`.
4. Call `POST /api/v1/platform/cpos` and persist the returned CPO ID, status,
   dummy app ID, app-ID mode, and administrator `identity_created` flag.
5. Call `POST /api/v1/platform/cpos/{cpo_id}/activate`.
6. The administrator calls login with `scope: CPO` and the returned CPO ID.
7. After OTP verification, inspect `must_change_password`.
8. If true, call `POST /api/v1/auth/password/change`; discard all returned
   session state because the successful change revokes it.
9. Login again, store the current refresh token, and call `/auth/me`.
10. Use `X-CPO-App-ID` only on CPO business routes.
11. When production app identity is known, the platform caller uses the
    app-ID `PUT` endpoint. CPO clients recover the new value through refresh or
    `/auth/me`.

Use the interactive page at `/docs/` to execute this flow. OTPs and generated
temporary passwords still arrive only by email; Swagger UI does not bypass the
security boundary.

## Frontend State Model

Recommended onboarding states:

```text
PLATFORM_LOGIN
-> PLATFORM_OTP
-> CPO_CREATED_PENDING
-> CPO_ACTIVE
-> CPO_ADMIN_LOGIN
-> CPO_ADMIN_OTP
-> PASSWORD_CHANGE_REQUIRED
-> REAUTHENTICATE
-> CPO_READY_DUMMY_APP_ID
-> CPO_READY_LIVE_APP_ID
```

Do not infer readiness from app-ID mode: an active CPO may legitimately use a
dummy ID. Do not infer lifecycle status from subscriptions: none are consulted.

## Failure and Recovery

- If mail is disabled, creation fails before any CPO or membership is written.
- A duplicate slug, GSTIN, app ID, or membership is rejected by durable
  constraints.
- Concurrent creation for the same new email is serialized so one global
  identity is created.
- A pending or suspended CPO cannot start a CPO login.
- App-ID rotation does not revoke sessions; clients recover the current value
  through refresh or `GET /api/v1/auth/me`.
- Suspension blocks new CPO login and revokes current CPO sessions and refresh
  tokens.
- Delivery jobs survive process restart and retry independently of the creating
  request.

## Operator Evidence

Safe evidence includes CPO ID/status/app-ID mode, admin user ID,
`identity_created`, outbox job status/attempts, session IDs, and audit action.
Never paste temporary passwords, OTPs, tokens, encrypted payloads, or SMTP
credentials into tickets or logs.

The exact request and response examples are in
`../../CPO_ADMINISTRATION.md`.
