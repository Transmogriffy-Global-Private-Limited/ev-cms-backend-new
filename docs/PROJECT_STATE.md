# Project State

## Current State

The repository began as an empty file scaffold. The implemented foundation now
provides:

- a compilable Go service;
- PostgreSQL connectivity and versioned migration execution;
- process-liveness and database-readiness endpoints;
- global identities;
- separate platform-superadmin records;
- CPO tenant organizations;
- fixed CPO-wide staff memberships;
- tenant-scoped customer relationships;
- user settings and tenant customer groups;
- hubs, chargers, connectors, favorites, and group access links;
- GST profiles and tariffs;
- wallets, wallet transactions, charging sessions, and wallet payments;
- platform and tenant audit logs;
- matching up and down migrations plus an explicit rollback operation;
- environment-only, concurrency-safe, idempotent initial-superadmin bootstrap;
- Argon2id passwords and bounded login lockout/rate limits;
- mandatory email OTP for platform and CPO administrative login;
- signed-then-encrypted access JWTs and rotating opaque refresh tokens;
- durable sessions with list/current/all/specific revocation APIs;
- enumeration-safe password recovery and authenticated password change;
- trusted principal, user ID, CPO ID, platform, and CPO-role helpers;
- encrypted PostgreSQL mail outbox with a retrying, encrypted-transport SMTP
  worker;
- write-only encrypted Razorpay credentials for CPO owners/admins;
- platform-only CPO create, list, inspect, activate, suspend, and app-ID APIs;
- subscription-independent pending CPO creation with unique dummy app IDs;
- transactional first-CPO-admin creation or safe existing-identity attachment;
- encrypted welcome email with CPO ID, app ID, and generated temporary
  password for new identities;
- non-expiring temporary-password change enforcement and login reminders;
- current dummy/live app identity in CPO login, refresh, and `me` responses;
- `X-CPO-App-ID` enforcement on tenant business APIs without trusting it as
  tenant authority;
- Hostinger implicit-TLS configuration on `smtp.hostinger.com:465`, with
  startup rejection of plaintext or ambiguous SMTP modes;
- registered educational, integration, API, internal-message, and
  configuration documentation under `docs/`.
- canonical OpenAPI 3.1 for all 40 implemented business/health operations;
- embedded same-origin Swagger UI at `/docs/` and raw OpenAPI at
  `/openapi.yaml`;
- bidirectional verification that Gin and OpenAPI expose the same operation
  set;
- public CPO-scoped customer signup start, verify, and resend APIs;
- durable signup challenges with hashed pending passwords and HMAC-protected
  OTPs;
- transactional global identity creation/reuse, tenant customer creation, and
  zero-balance INR wallet creation;
- a separate `CUSTOMER` session scope bound to one global user, customer, and
  CPO without a staff role;
- customer password-plus-mail-OTP login, signed/encrypted access tokens, and
  rotating/reuse-detecting refresh tokens;
- app-user `me`, customer-scoped session listing/revocation/logout, and global
  password recovery/change;
- trusted backend current-principal, user, customer, CPO, and app-ID helpers.

No inventory API, HAL integration, charging workflow, billing orchestration,
payment workflow, or reporting behavior is implemented yet.

## Verification

- Go formatting completed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- The embedded up migration created all 21 domain tables in a disposable local
  PostgreSQL 17 database.
- Reapplying up was idempotent and retained one migration version.
- The matching down migration removed all domain tables and retained only the
  migration ledger.
- The auth migration rolled down, up, and idempotently up in PostgreSQL 17.
- Bootstrap twice retained one platform admin and did not overwrite its
  password.
- Platform and CPO admin email-OTP login passed using encrypted outbox payloads.
- Encrypted access-token validation, refresh rotation, reuse-triggered session
  revocation, and password recovery passed.
- The mail worker claimed, decrypted, delivered through a test sender, and
  completed a durable job.
- Hostinger implicit-TLS configuration loaded successfully and the SMTP sender
  construction tests passed without exposing a real mailbox password.
- The required documentation, administrative route coverage, and removed SMTP
  configuration residue checks passed.
- OpenAPI parsing, semantic validation, all 40 Gin/OpenAPI operation matches,
  and docs/raw-spec HTTP smoke tests passed.
- Razorpay secrets remained encrypted in storage, resolved only internally for
  the correct CPO, and were unavailable to a platform principal.
- Route registration, unauthenticated rejection, request validation, and
  no-store behavior passed.
- The CPO provisioning migration rolled down, up, and idempotently up in
  PostgreSQL 17.
- CPO creation without subscriptions, encrypted temporary-password delivery,
  concurrent and sequential identity reuse, repeated-login reminders,
  business-API password gate, password change, activation, dummy-to-live
  app-ID rotation, and suspension passed in PostgreSQL lifecycle tests.
- The customer-signup migration rolled down, up, and idempotently up in
  PostgreSQL 17.
- Customer signup resend rotation, old/replayed challenge rejection, new
  identity/password creation, cross-CPO identity reuse without credential or
  profile overwrite, and zero-balance INR wallet creation passed in a
  PostgreSQL lifecycle test.
- The customer-authentication migration rolled down, up, and idempotently up
  in PostgreSQL 17.
- Customer login OTP, encrypted access validation, `me`, refresh rotation and
  reuse revocation, customer-scoped session listing/revocation/logout,
  password recovery, password change, and global session revocation passed in
  PostgreSQL lifecycle tests.

## Current Access Model

- `users` represent login identities.
- `platform_admins` explicitly grant platform-superadmin authority.
- `cpos` represent tenant/customer organizations.
- `cpo_memberships` grant a fixed role inside one CPO.
- `customers` represent a user's customer relationship with one CPO.

The full mapping from the supplied schema is recorded in `docs/SCHEMA.md`.

The same identity may belong to multiple CPOs. Its staff and customer
relationships remain distinct and tenant-scoped.

An administrative session selects exactly one platform or CPO scope. Protected
requests revalidate the durable session and current authority. Tenant context
comes from that principal rather than a request header. Tenant business routes
also verify that `X-CPO-App-ID` equals the current dummy or live ID for that
same principal; the app ID never grants authority or changes scope.

## HAL Boundary

The HAL remains a separate service and database. It is not embedded in this
repository. The integration contract has not been implemented yet.

## Known Limitations

- Domain tables have no repositories or handlers yet.
- CPO staff invitation after the first admin and customer email/profile-change
  workflows are not implemented.
- Automatic encryption-key rotation is not implemented; data must be
  re-encrypted before an old key is removed.
- SMTP delivery logic is implemented and worker-tested, but real Hostinger
  delivery remains operationally unverified. The mailbox password is not stored
  in the repository.
- No generated frontend SDK exists yet; consumers use the reviewed OpenAPI
  contract directly.
