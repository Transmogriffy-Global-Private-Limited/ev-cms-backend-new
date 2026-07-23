# Mail Outbox Internal Contract

## Producer Contract

Mail-producing application transactions call the outbox with a normalized
recipient, template name, and structured payload. The encrypted outbox row is
committed in the same database transaction as the state that requires the
message.

Supported templates and payload fields:

| Template | Required meaningful fields |
|---|---|
| `LOGIN_OTP` | recipient name, code, expiry |
| `PASSWORD_RESET_OTP` | recipient name, code, expiry |
| `CPO_ADMIN_WELCOME` | recipient name, temporary password, CPO name, CPO ID, app ID |
| `CPO_MEMBERSHIP_ASSIGNED` | recipient name, CPO name, CPO ID, app ID |
| `PASSWORD_CHANGE_REMINDER` | recipient name |
| `CUSTOMER_SIGNUP_OTP` | recipient name, code, expiry |
| `CUSTOMER_LOGIN_OTP` | recipient name, code, expiry |
| `CUSTOMER_PASSWORD_RESET_OTP` | recipient name, code, expiry |

The JSON payload is encrypted with AES-256-GCM and authenticated using:

```text
ev-cms-mail:<template>:<normalized-recipient>
```

The database retains ciphertext and an encryption key ID, not payload
plaintext.

## Stored Job Fields

Each row records job ID, normalized recipient, template, encrypted payload,
encryption key ID, status, attempts, maximum attempts, availability, lock time,
bounded last error, sent time, and creation/update timestamps. The recipient
and template remain queryable operational metadata; sensitive message content
does not.

## State Machine

```text
PENDING -> PROCESSING -> SENT
              |
              +-> PENDING (retry available later)
              |
              +-> FAILED (attempt limit reached)
```

- New jobs permit eight attempts.
- Workers claim one eligible job using `FOR UPDATE SKIP LOCKED`.
- A `PROCESSING` lock older than five minutes can be reclaimed.
- Retry begins at one minute, doubles per attempt, and caps at one hour.
- `SENT` and `FAILED` are terminal in the current worker.
- Stored delivery errors are truncated to 500 characters.

Attempt count increments at claim time, not after SMTP failure. A process crash
during an attempt therefore still consumes an attempt when the job is later
reclaimed.

## Delivery Semantics

The contract is at-least-once, not exactly-once. If SMTP accepts a message and
the worker crashes before recording `SENT`, the stale job can be reclaimed and
sent again. Templates and consuming workflows must tolerate duplicate email.
OTP and recovery authority remains in the single-use database challenge, not
in the email itself.

The worker decrypts only after claiming a row and holds plaintext only in
memory for rendering and sending. Missing or wrong encryption-key IDs fail
closed.

## Producer Transaction Rule

A producer must receive the caller's active GORM transaction and insert the job
before that transaction commits. Enqueuing after the business commit would
permit durable challenges/onboarding without durable delivery intent; sending
before commit would permit email for state that later rolled back.

## Worker Concurrency

Multiple application processes may run workers. `FOR UPDATE SKIP LOCKED` and
the atomic status update ensure one live claim at a time. The five-minute stale
threshold is recovery, not a lease-renewal protocol; `MAIL_SEND_TIMEOUT` must
remain materially below it.

## Template Compatibility

Template name and payload fields form an internal message contract. Producers,
renderer, tests, and this document must change together. Removing an old
renderer while old encrypted jobs remain pending would make those jobs
undeliverable.

## Operational Gaps

There is no administrative replay endpoint, separate dead-letter store, or
automated re-encryption workflow. Operators may inspect safe metadata but must
not expose encrypted payloads, SMTP passwords, OTPs, or temporary passwords.
