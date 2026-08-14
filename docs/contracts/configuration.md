# Configuration Contract

## Loading and Precedence

The service attempts to load `.env`; existing process environment values take
precedence because they are not overwritten. `.env.example` is the checked-in
inventory. Real secrets belong only in the ignored `.env`, process environment,
or deployment secret store.

Configuration is loaded and fully validated before database connection,
migrations, bootstrap, workers, or HTTP listening. A configuration error is a
startup failure; the server does not run partially.

`API_DOCS_ENABLED=true` exposes the embedded interactive Swagger UI and raw
authoritative OpenAPI contract. `false` leaves all ordinary API and health
routes available but does not register any documentation route, so each returns
the normal Gin `404`. The setting contains no secret but should normally be
disabled where exposing API shape is not operationally desired. Changes require
a process restart.

Safe JSON HTTP completion and panic logging is always enabled and writes to
stdout; the process supervisor owns capture and retention. `LOG_LEVEL` defaults
to `INFO` and accepts only `INFO` or `DEBUG`. `DEBUG` adds safe request-start
and handled-error type/classification/SQL-state events; it never enables bodies,
raw URLs, queries, headers, secrets, personal fields, or raw error values. A
change requires restart. The fields, proxy trust, and mandatory exclusions are
defined in `internal/http-request-logging.md`.

## Core and Bootstrap

| Variable | Requirement / default |
|---|---|
| `DATABASE_URL` | Required |
| `HTTP_ADDR` | Code and development example default `127.0.0.1:8080`; do not bind local development to a public interface. |
| `CORS_ALLOW_ALL` | Default and development example `false`; use an explicit approved origin when cross-origin development is required. |
| `API_DOCS_ENABLED` | Boolean, default `true`; controls registration of `/docs`, `/docs/`, and `/openapi.yaml` |
| `LOG_LEVEL` | Optional; `INFO` default. `DEBUG` enables additional safe developer diagnostics; no other value is accepted. |
| `SUPERADMIN_EMAIL` | Required valid email |
| `SUPERADMIN_PASSWORD` | Required, 10 to 128 characters; existing password is never overwritten |
| `SUPERADMIN_FULL_NAME` | Default `Platform Superadmin` |

## Authentication

| Variable | Default / validation |
|---|---|
| `AUTH_ISSUER` | `ev-cms` |
| `AUTH_AUDIENCE` | `ev-cms-api` |
| `AUTH_ACCESS_TTL` | `15m`, positive |
| `AUTH_SESSION_TTL` | `720h`, longer than access TTL |
| `AUTH_OTP_TTL` | `10m`, positive |
| `AUTH_OTP_RESEND_COOLDOWN` | `1m`, positive |
| `AUTH_LOGIN_MAX_ATTEMPTS` | `5`, positive |
| `AUTH_LOGIN_LOCK_DURATION` | `15m` |
| `AUTH_RATE_LIMIT_WINDOW` | `15m` |
| `AUTH_RATE_LIMIT_MAX` | `100`, positive |

Durations use Go duration syntax.

Invalid duration text is converted to a validation failure. Session TTL must be
strictly longer than access TTL. Attempt/rate limits and mail worker durations
must be positive.

## Platform Operations and Realtime

| Variable | Default / validation |
|---|---|
| `PLATFORM_EVENT_RETENTION` | `168h`; positive durable event retention |
| `PLATFORM_REALTIME_POLL_INTERVAL` | `1s`; positive PostgreSQL catch-up interval |
| `PLATFORM_REALTIME_HEARTBEAT_INTERVAL` | `15s`; must exceed the polling interval |
| `PLATFORM_REALTIME_BATCH_SIZE` | `100`; integer from 1 through 500 |
| `PLATFORM_WORKER_STALE_AFTER` | `2m`; positive and longer than `PLATFORM_MAINTENANCE_INTERVAL` |
| `PLATFORM_MAINTENANCE_INTERVAL` | `1m`; positive event-cleanup and maintenance heartbeat interval |

The platform-maintenance worker deletes expired event rows and reports a
durable heartbeat. The mail worker also reports durable heartbeats when mail is
enabled. Readiness is evaluated per required worker name: at least one instance
must be fresh and healthy, so a stale instance from a replaced process does not
mask a healthy replacement. Realtime and retention configuration is loaded at
startup and requires a restart to change.

## Cryptographic Keys

| Variable | Requirement |
|---|---|
| `JWT_SIGNING_KEY_B64` | Standard base64 decoding to at least 32 bytes |
| `JWT_ENCRYPTION_KEY_B64` | Standard base64 decoding to exactly 32 bytes |
| `OTP_HMAC_KEY_B64` | Standard base64 decoding to at least 32 bytes |
| `MAIL_OUTBOX_ENCRYPTION_KEY_B64` | Standard base64 decoding to exactly 32 bytes |
| `MAIL_OUTBOX_ENCRYPTION_KEY_ID` | Default `v1` |
| `CREDENTIAL_ENCRYPTION_KEY_B64` | Standard base64 decoding to exactly 32 bytes |
| `CREDENTIAL_ENCRYPTION_KEY_ID` | Default `v1` |

Use independent random keys. A key ID is not a key. Existing encrypted rows
must be re-encrypted before an old key is removed; automatic rotation is not
implemented.

## OCPP HAL V1

| Variable | Requirement / default |
|---|---|
| `HAL_V1_BASE_URL` | Optional. Base URL for the independently deployed HAL; no trailing slash. Leave unset to keep customer charging unavailable rather than guessing a provider. Configure a loopback URL only when the approved v1 provider is running locally. |
| `HAL_V1_CMS_BEARER_TOKEN` | Secret. Required with `HAL_V1_BASE_URL`; CMS-to-HAL command and mapping bearer. |
| `HAL_V1_CMS_FACT_BEARER_TOKEN` | Secret. Required with `HAL_V1_BASE_URL`; HAL-to-CMS fact bearer, distinct from the command bearer. |
| `HAL_V1_REQUEST_TIMEOUT` | `5s`, positive HTTP client timeout. A timeout is reconciliation evidence, not permission to create another command. |
| `HAL_V1_METER_STALE_AFTER` | `30s`, positive CMS display freshness threshold; it never creates an inferred meter value. |
| `HAL_V1_CONNECTION_STALE_AFTER` | `15m`, positive CMS connection-liveness horizon. It must remain comfortably longer than HAL's requested Heartbeat cadence (v1 default `300s`); it never creates or infers `ONLINE`. |

Changing any HAL setting requires restart. Never substitute a legacy HAL token,
customer bearer, staff bearer, or a shared database connection. See
`integrations/ocpp-hal-boundary.md` for ownership and recovery rules.

## Mail and Hostinger

| Variable | Requirement / current value |
|---|---|
| `MAIL_ENABLED` | Default false; must be true for administrative login and recovery |
| `SMTP_HOST` | Required when enabled; current `smtp.hostinger.com` |
| `SMTP_PORT` | 1-65535; current `465` |
| `SMTP_USERNAME` | Current `team@transev.in`; must be paired with password |
| `SMTP_PASSWORD` | Secret; required when username is set |
| `SMTP_FROM_ADDRESS` | Required valid email; current `team@transev.in` |
| `SMTP_FROM_NAME` | Default `TransEV CMS` |
| `SMTP_USE_SSL` | Implicit TLS from connection start |
| `SMTP_USE_TLS` | Mandatory STARTTLS |
| `MAIL_WORKER_POLL_INTERVAL` | Default `2s`, positive |
| `MAIL_SEND_TIMEOUT` | Default `15s`, positive |

When mail is enabled, exactly one of `SMTP_USE_SSL` or `SMTP_USE_TLS` must be
true. The current Hostinger port-465 contract is:

```dotenv
SMTP_USE_TLS=false
SMTP_USE_SSL=true
```

`SMTP_TLS_MODE` is removed and ignored. Deployments must migrate to the two
explicit boolean flags. Plaintext SMTP is intentionally unavailable.

## Secret Handling

Never log, commit, document, or return:

- superadmin or SMTP passwords;
- signing, encryption, or HMAC keys;
- OTPs, refresh tokens, or access tokens;
- Razorpay secrets.

The non-secret Hostinger username, sender address, host, port, and transport
selection may be checked in.

## Rotation and Deployment Procedure

- JWT signing/encryption key changes invalidate existing access tokens.
- Refresh tokens and durable sessions remain database records, but a client
  needs a newly issued access token under the available keys.
- Mail/credential encryption keys use key IDs stored beside ciphertext.
- Automatic multi-key lookup and re-encryption are not implemented.
- Do not remove an old encryption key until a reviewed re-encryption operation
  has migrated every referenced row.
- Restart is required for environment changes; configuration is not hot
  reloaded.

## Failure Reference

| Failure | Meaning |
|---|---|
| required variable error | Missing startup dependency |
| base64 error | Key is not standard base64 |
| decoded-length error | Cryptographic key has wrong byte length |
| TTL relationship error | Session lifetime cannot safely contain access lifetime |
| SMTP transport error | Neither or both encrypted modes selected |
| SMTP credential-pair error | Username/password only partly configured |
| email validation error | Bootstrap or sender address is not a normalized valid address |

## Local Development Network Safety

The checked-in development example binds `127.0.0.1:8080` and keeps CORS
restricted. Local dual-service acceptance also uses loopback addresses. Do not
change a local checkout to `0.0.0.0` or wildcard CORS merely for convenience.
