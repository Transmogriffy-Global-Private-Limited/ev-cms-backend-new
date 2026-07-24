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

## Core and Bootstrap

| Variable | Requirement / default |
|---|---|
| `DATABASE_URL` | Required |
| `HTTP_ADDR` | Code default `127.0.0.1:8080`; current development example uses `0.0.0.0:8080` for access from other machines |
| `CORS_ALLOW_ALL` | Default `false`; current development example sets `true` to allow every browser origin and requested header |
| `API_DOCS_ENABLED` | Boolean, default `true`; controls registration of `/docs`, `/docs/`, and `/openapi.yaml` |
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
| `PLATFORM_WORKER_STALE_AFTER` | `30s`; positive heartbeat staleness threshold |
| `PLATFORM_MAINTENANCE_INTERVAL` | `1m`; positive event-cleanup and maintenance heartbeat interval |

The platform-maintenance worker deletes expired event rows and reports a
durable heartbeat. The mail worker also reports durable heartbeats when mail is
enabled. Realtime and retention configuration is loaded at startup and requires
a restart to change.

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

## Temporary Unrestricted Development Access

The current `.env.example` intentionally opts into:

```dotenv
HTTP_ADDR=0.0.0.0:8080
CORS_ALLOW_ALL=true
API_DOCS_ENABLED=true
```

`HTTP_ADDR=0.0.0.0:8080` listens on every IPv4 interface.
`CORS_ALLOW_ALL=true` returns `Access-Control-Allow-Origin: *`, accepts browser
preflight requests for the API methods, and reflects requested preflight
headers. It does not disable authentication, app-ID validation, tenant scope,
or authorization.

Clients on another machine use `http://<server-ip>:8080`. This temporary mode
must be replaced with an explicit origin allowlist and an HTTPS reverse proxy
before production exposure. Set `HTTP_ADDR=127.0.0.1:8080` and
`CORS_ALLOW_ALL=false` to restore loopback-only, same-origin behavior.
