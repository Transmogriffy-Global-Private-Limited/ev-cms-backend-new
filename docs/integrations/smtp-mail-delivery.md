# SMTP Mail Delivery Integration

## Purpose and Ownership

The CMS uses Hostinger SMTP for administrative OTP, password recovery, CPO
onboarding, membership assignment, and temporary-password reminders.
PostgreSQL owns durable delivery intent in `mail_outbox`; SMTP owns only the
external delivery attempt.

## Provider Contract

The current deployment settings are:

```dotenv
MAIL_ENABLED=true
SMTP_HOST=smtp.hostinger.com
SMTP_PORT=465
SMTP_USERNAME=team@transev.in
SMTP_PASSWORD=<environment-only secret>
SMTP_FROM_ADDRESS=team@transev.in
SMTP_FROM_NAME=TransEV CMS
SMTP_USE_TLS=false
SMTP_USE_SSL=true
```

Port 465 uses implicit TLS: encryption begins when the connection opens.
`SMTP_USE_TLS=true` instead means mandatory STARTTLS and must be paired with
`SMTP_USE_SSL=false`. Exactly one mode must be true while mail is enabled.
Plaintext SMTP is not supported.

`SMTP_USERNAME` and `SMTP_PASSWORD` must be supplied together. The password
must remain in the ignored `.env`, process environment, or a deployment secret
store. It must not appear in source, documentation, tests, logs, or API output.

### Transport-selection matrix

| `SMTP_USE_SSL` | `SMTP_USE_TLS` | Meaning | Accepted |
|---:|---:|---|---:|
| true | false | Implicit TLS from connection open; Hostinger 465 | Yes |
| false | true | Plain connection upgraded with mandatory STARTTLS | Yes |
| false | false | Plaintext-capable SMTP | No |
| true | true | Ambiguous double selection | No |

Hostinger credentials authenticate the SMTP session; they do not authenticate
CMS HTTP users. `SMTP_FROM_ADDRESS` controls the message sender and is validated
as an email during startup.

## Delivery Flow

```text
auth or onboarding transaction
  -> encrypted mail_outbox row (PENDING)
  -> worker claims row (PROCESSING)
  -> decrypt in worker memory
  -> Hostinger implicit-TLS SMTP
  -> SENT, or bounded retry / FAILED
```

Jobs are claimed with `FOR UPDATE SKIP LOCKED`. Processing locks older than
five minutes are reclaimable. The default worker poll is two seconds, the
default send timeout is 15 seconds, and a job gets eight attempts. Retry delay
starts at one minute, doubles, and is capped at one hour.

This is at-least-once delivery. A crash after Hostinger accepts a message but
before PostgreSQL records `SENT` can produce a duplicate. Authentication
challenges remain single-use, and recipients should treat repeated notices as
duplicates.

## Message Classes

| Template | Producer | Contains sensitive plaintext before encryption |
|---|---|---|
| `LOGIN_OTP` | Administrative login | OTP and expiry |
| `PASSWORD_RESET_OTP` | Password recovery | OTP and expiry |
| `CPO_ADMIN_WELCOME` | New first-admin onboarding | Generated temporary password, CPO/app IDs |
| `CPO_MEMBERSHIP_ASSIGNED` | Existing-identity onboarding | CPO/app IDs |
| `PASSWORD_CHANGE_REMINDER` | Login while temporary password remains | No credential |

The SMTP sender renders plain-text email. Templates are code-owned; callers
provide structured payloads rather than arbitrary subject/body HTML.

## Failure Behavior

- Commands requiring new mail fail safely with `mail_unavailable` when mail is
  disabled.
- Provider and network errors return the job to `PENDING` until attempts are
  exhausted.
- `FAILED` is terminal in the current implementation; there is no separate
  dead-letter table or replay API.
- Stored error text is bounded to 500 characters.
- Missing encryption-key versions fail delivery rather than guessing a key.
- Mail payload plaintext exists only during command construction and worker
  delivery.

### Crash windows

- Before transaction commit: neither business state nor mail job exists.
- After commit, before claim: the pending job survives restart.
- After claim, before SMTP acceptance: a stale lock is reclaimed after five
  minutes.
- After SMTP acceptance, before `SENT`: reclaim may send a duplicate.
- After `SENT`: the normal worker will not claim the job again.

The database cannot atomically commit with an external SMTP provider, so
exactly-once email delivery is not claimed.

## Operational Checks

Safe fields for support are job ID, normalized recipient where authorized,
template, status, attempt count, maximum attempts, timestamps, and bounded last
error. Never expose payload ciphertext as a workaround, because it may contain
OTP or temporary-password material after decryption.

The readiness endpoint currently checks PostgreSQL, not SMTP provider
availability. A healthy readiness response therefore does not prove Hostinger
can accept mail.

## Verification Boundary

Configuration validation and implicit-TLS SMTP client construction are covered
by tests. The outbox worker has PostgreSQL-backed delivery tests using a fake
sender. A live Hostinger delivery has not been executed by the repository test
suite and must not be claimed until an authorized environment performs it.

An authorized live smoke test should use a non-production recipient, confirm
sender/domain acceptance, confirm TLS and authentication, observe the outbox
transition to `SENT`, and remove test artifacts through an approved data-safe
process. Never add the mailbox password to a command captured in shell history.

Hostinger documents `smtp.hostinger.com` with SSL port 465 and TLS/STARTTLS
port 587 in its email-client configuration guidance:
<https://support.hostinger.com/en/articles/1575756-how-to-get-email-account-configuration-details-for-hostinger-email>.
