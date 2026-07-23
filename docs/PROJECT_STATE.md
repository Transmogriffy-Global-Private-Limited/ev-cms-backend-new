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
- encrypted PostgreSQL mail outbox with retrying SMTP TLS worker;
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
- API and operational documentation in `docs/AUTHENTICATION.md` and
  `docs/CPO_ADMINISTRATION.md`.

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
- Public customer login, signup, CPO staff invitation after the first admin,
  and email-change workflows are not implemented.
- Automatic encryption-key rotation is not implemented; data must be
  re-encrypted before an old key is removed.
- SMTP delivery logic is implemented and worker-tested, but no real provider
  delivery was attempted because provider credentials were not supplied.
