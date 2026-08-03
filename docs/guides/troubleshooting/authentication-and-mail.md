# Troubleshooting Authentication and Mail

Use these checks without printing passwords, OTPs, tokens, encrypted payloads,
or full provider errors containing sensitive values.

## Start With the Layer That Failed

| Symptom | Likely layer | First safe check |
|---|---|---|
| Process exits before listening | Configuration | Startup error and `.env.example` names |
| `/health/live` fails | HTTP process | Listener address, server IP, and firewall |
| live works, ready is 503 | PostgreSQL | `DATABASE_URL` target and database reachability |
| login is 503 | Mail feature | `MAIL_ENABLED` and SMTP validation |
| login is 202, no email | Outbox/SMTP | Job status, attempts, bounded error |
| OTP returns 401 | Challenge | Newest challenge, expiry, resend/attempt state |
| bearer returns 401 | Token/session | Token expiry and durable revocation |
| tenant operation returns 403 | Role/password/app ID | Error code, `/auth/me`, current app ID |

## Interactive API Page Does Not Load

- Confirm `API_DOCS_ENABLED=true`; when false, `/docs`, `/docs/`, and
  `/openapi.yaml` intentionally return `404`.
- `/docs` should return a temporary redirect to `/docs/`.
- `/docs/` and its assets are embedded; no CDN is required.
- `/openapi.yaml` should return the specification.
- With `CORS_ALLOW_ALL=true`, remote browser origins and their requested
  preflight headers are accepted. Without it, use the same origin as the API.
- Use Swagger UI **Authorize** for bearer and CPO app-ID values. Do not paste a
  refresh token into the bearer field.

## Service Does Not Start

If startup reports that exactly one SMTP transport must be enabled:

- Hostinger port 465 requires `SMTP_USE_SSL=true` and
  `SMTP_USE_TLS=false`.
- A provider using mandatory STARTTLS requires the inverse.
- Both false would permit plaintext SMTP and is rejected.
- Both true is ambiguous and is rejected.

If startup reports paired credentials, set both `SMTP_USERNAME` and
`SMTP_PASSWORD`, or neither. For the deployed Hostinger mailbox, keep
`team@transev.in` in the username/from fields and place the password only in the
ignored `.env` or process environment.

## Login Returns `mail_unavailable`

Administrative login and recovery require `MAIL_ENABLED=true`. Confirm the
worker was constructed, the database is reachable, and the Hostinger values
match `../../integrations/smtp-mail-delivery.md`. Do not bypass email OTP by
returning or logging the code.

## OTP Email Has Not Arrived

1. Confirm the API returned `202 Accepted`.
2. Inspect only safe outbox metadata: job ID, recipient address as authorized,
   template, status, attempt count, available time, and bounded last error.
3. `PENDING` with a future `available_at` is waiting for retry.
4. `PROCESSING` older than five minutes is reclaimable by a worker.
5. `FAILED` after eight attempts is terminal and needs an operator decision.
6. Check spam filtering and the sender mailbox after transport configuration is
   confirmed.

The worker can deliver a message more than once if SMTP accepts it and the
process crashes before `SENT` is recorded. OTP challenges remain single-use.

For administrative password recovery, use both values in the current email:
the opaque recovery ID and six-digit code. An email generated before recovery
ID delivery was implemented cannot complete reset; request a fresh email rather
than retrieving challenge state from the database.

## Token or Session Is Rejected

- `unauthorized`: token missing, invalid, expired, revoked, or durable authority
  no longer valid.
- `invalid_refresh_token`: refresh token is invalid or already consumed. Reuse
  revokes its session.
- `password_change_required`: use the authenticated password-change endpoint or
  password recovery, then sign in again.
- `missing_cpo_app_id`: add the current `X-CPO-App-ID` on a tenant business
  request.
- `cpo_app_id_mismatch`: recover the current app ID through refresh or
  `GET /api/v1/auth/me`; never use the header to switch CPOs.

## Login and Challenge Edge Cases

- `invalid_credentials` intentionally combines wrong password, missing
  platform authority, invalid CPO membership, inactive CPO, inactive identity,
  and lockout. Do not build UI that claims which one occurred.
- `invalid_challenge` can mean malformed OTP, expired challenge, consumed code,
  too many attempts, resend cooldown, or an invalidated earlier challenge.
- A resend returns a new challenge ID. Replace both the challenge ID and the
  OTP-entry context in the client.
- Password change/reset revokes sessions. A following 401 is expected; send the
  user through login rather than retrying the old token.

## App-ID Problems

Call `/api/v1/auth/me` or refresh to obtain the current `cpo_app_id` and mode.
Do not cache the onboarding dummy value permanently. A platform app-ID rotation
does not revoke the session, so the first visible symptom may be
`cpo_app_id_mismatch` on a tenant operation.

## When a Job Is `FAILED`

Eight delivery attempts are exhausted. The current system has no replay API or
dead-letter table. Preserve safe metadata and investigate the provider/config
cause. Do not directly edit ciphertext or reset attempts in an unknown
database. A replay/reconciliation tool requires a separately reviewed feature.

## Safe Local Verification

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.local/go-cache'
go test ./src/config ./src/mail -count=1
go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1
go test ./src/routes -run TestAPIDocumentationRoutesCanBeDisabled -count=1
.\scripts\verify-docs.ps1
```

These checks validate configuration and code paths; they do not send through
the real Hostinger mailbox.
