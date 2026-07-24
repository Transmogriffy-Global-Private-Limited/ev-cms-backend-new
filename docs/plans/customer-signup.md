# Customer Signup Plan

Status: Verified

## Objective

Allow a customer to register under one active CPO through that CPO's mobile or
web application without weakening the global-identity and tenant boundaries.

## Contract

- `POST /api/v1/app/auth/signup` validates the current `X-CPO-App-ID`, customer
  profile, and password, then mails a durable verification challenge.
- `POST /api/v1/app/auth/signup/verify` consumes the challenge and atomically
  creates or reuses the global identity, creates the CPO-scoped customer, and
  creates its zero-balance INR wallet.
- `POST /api/v1/app/auth/signup/resend` rotates an eligible challenge after its
  cooldown.
- The app ID is public routing metadata. It selects only an active CPO and is
  never treated as authentication or abuse prevention.

## Security Invariants

- No unverified `users` row is created, so abandoned signups cannot squat a
  global email.
- Pending challenges store an Argon2id password hash, never the submitted
  plaintext password. Terminal verification and resend paths scrub the obsolete
  challenge copy.
- OTPs are HMAC-protected, single-use, expiring, attempt-bounded, and delivered
  through the encrypted durable mail outbox.
- Start and verification operations are database-rate-limited.
- Existing active global identities are attached only after mailbox
  verification; their password, name, and phone are not overwritten.
- Inactive global identities cannot be attached.
- Verification is serialized by normalized email and all durable writes occur
  in one PostgreSQL transaction.
- CPO status and app ID are revalidated at every step.

## Non-goals

- Customer login, access/refresh tokens, social login, SMS OTP, or app attestation
- Commercial access or payment checks
- Customer profile editing
- Automatic assignment to a customer group

## Acceptance Criteria

- A valid challenge under an active CPO creates exactly one customer and wallet.
- Repeated or concurrent verification cannot duplicate identities, customers,
  or wallets.
- One global identity may become a customer of multiple CPOs.
- Existing credentials are preserved when an identity is reused.
- Invalid, expired, consumed, over-attempted, cross-CPO, or inactive-CPO
  challenges fail safely.
- Runtime routes, OpenAPI, human API documentation, migrations, models, and
  verification agree.

## Verification

- Unit tests for validation, OTP binding, and route behavior
- PostgreSQL lifecycle coverage for new identity, identity reuse, wallet
  creation, replay, and tenant isolation
- Migration up/down pairing and schema-model parsing
- OpenAPI/runtime drift verification
- Full Go tests, vet, documentation verification, and Git whitespace checks

Completed evidence:

- PostgreSQL lifecycle covered resend rotation, replay rejection, identity
  creation, identity reuse without overwrite, tenant separation, and wallet
  creation.
- Migration 000004 passed down, up, and idempotent-up execution.
- All 27 runtime/OpenAPI operations matched.
- Documentation verification, `go test ./...`, and `go vet ./...` passed.
