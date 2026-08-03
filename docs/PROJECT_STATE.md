# Project State

## Current State

The repository began as an empty file scaffold. The implemented foundation now
provides:

- a compilable Go service;
- PostgreSQL connectivity and versioned migration execution;
- process-liveness and database-readiness endpoints;
- always-on JSON HTTP completion logging with server-generated request IDs,
  matched route templates, result/latency/size fields, safe authenticated
  identifiers, handled API error codes, safe correlated panic stack
  diagnostics, and explicit secret/content exclusion;
- optional `LOG_LEVEL=DEBUG` request-start and handled-error
  component/type diagnostics under the same server request ID;
- global identities;
- separate platform-superadmin records;
- CPO tenant organizations;
- CPO membership persistence with ADMIN as the only callable tenant authority;
  OWNER, OPERATOR, and VIEWER remain dormant future-compatible enum values;
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
  eligible reset mail delivers the recovery ID, code, and expiry required by
  the reset handler while the forgot response remains generic;
- trusted principal, user ID, CPO ID, platform, and CPO-role helpers;
- encrypted PostgreSQL mail outbox with a retrying, encrypted-transport SMTP
  worker;
- write-only encrypted Razorpay credentials for CPO admins;
- platform-only CPO create, searchable/filterable/cursor list, inspect, profile,
  reasoned activate/suspend, and app-ID APIs;
- required GSTIN plus complete address fields for CPO creation/profile
  replacement, backed by database constraints and normalized GSTIN uniqueness;
- authenticated platform slug-availability lookup for responsive FE validation,
  with final creation/database uniqueness remaining authoritative;
- constraint-aware platform CPO conflict responses that distinguish slug,
  GSTIN, app ID, administrator identity, membership, and primary-admin races;
- durable current lifecycle reason, actor, and transition time;
- one durable primary administrator per provisioned CPO, with safe visibility,
  replacement/restoration, credential-free onboarding resend, and targeted CPO
  administrative-session revocation;
- manually controlled pending CPO creation with unique dummy app IDs;
- transactional first-CPO-admin creation or safe existing-identity attachment;
- encrypted welcome email with CPO ID, app ID, and generated temporary
  password for new identities;
- non-expiring temporary-password change enforcement and login reminders;
- current dummy/live app identity in CPO login, refresh, and `me` responses;
- `X-CPO-App-ID` enforcement on tenant business APIs without trusting it as
  tenant authority;
- CPO ADMIN identity-profile read/update for global full-name and phone fields;
- session-bound, read-only CPO registration/organization details without
  exposing internal Superadmin actor metadata or permitting tenant mutation;
- tenant-scoped bounded hub create/list/get/update;
- atomic CMS charger/connector registration with server-generated charger UUID,
  public charger ID, OCPP mapping identity, and connector UUIDs;
- bounded charger listing, detail/update, connector update, and dependency-safe
  charger deletion;
- exact-decimal, bounded GST and tariff create/list/get/update with cross-CPO
  relationship rejection and INR defaulting;
- Hostinger implicit-TLS configuration on `smtp.hostinger.com:465`, with
  startup rejection of plaintext or ambiguous SMTP modes;
- registered educational, integration, API, internal-message, and
  configuration documentation under `docs/`;
- a canonical CPO backend AI-agent handoff covering current capability,
  ownership, tenant/HAL boundaries, remaining dependency order, slice
  execution, verification, and handoff requirements;
- a canonical SuperAdmin frontend handoff covering the 28-operation platform
  integration surface, TypeScript contracts, auth/token state, CPO workflows,
  audit/workers, SSE/replay, error UX, security, verification, and explicit
  blocked/unimplemented behavior;
- canonical OpenAPI 3.1 for all 70 source-tree business/health operations;
- embedded same-origin Swagger UI at `/docs/` and raw OpenAPI at
  `/openapi.yaml`;
- `API_DOCS_ENABLED` registration control for both documentation surfaces,
  defaulting on for compatibility and returning `404` when disabled;
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
- app-user `me`, customer-scoped session listing/revocation/logout, global
  password reset/change, and eligible-recipient recovery-ID/code delivery;
- trusted backend current-principal, user, customer, CPO, and app-ID helpers;
- environment-controlled permissive CORS middleware and a current development
  configuration that listens on all IPv4 interfaces for access from other
  machines;
- durable platform event replay, authenticated SSE, filtered audit query, and
  registered worker-health/readiness APIs;
- manual superadmin CPO access through explicit activation and suspension, with
  no tenant subscription, entitlement, platform-invoice, or platform-payment
  runtime;
- a reversible migration-nine retirement boundary that preserves the removed
  prototype tables in `retired_commercial` and disables their worker records;
- an active VPS deployment at `dev-evcmsnew.transev.site`, with Caddy proxying
  to the loopback-only listener `127.0.0.1:18080`;
- an enabled and active `evcmsnew-dev.service`, ignored mode-0600 deployment
  environment, compiled binary layout, and `rehost-evcmsnew` interactive
  handler;
- the additive PostgreSQL database `devevcmsnewdb`, owned by `postgres`.

The active development VPS runs source revision `d27e599`, with migration
eleven recorded and the deployed 70-operation contract. CPO GSTIN and address
identity are database-required, authenticated platform clients can use the
advisory slug-availability operation, and known uniqueness races return
field- or relationship-specific conflict codes. All four CPOs were complete
and preserved during deployment. Migration nine continues to preserve the
`retired_commercial` schema. Safe structured HTTP request logging is active;
the current development environment uses `LOG_LEVEL=DEBUG` for correlated
request-start and completion diagnostics.

No CMS/HAL transport or handshake, live charger state ingestion, charging
workflow, tenant payment workflow, tenant commercial-management workflow,
staff-management workflow, or reporting behavior is implemented
yet.

## Verification

- Go formatting completed.
- Safe request completion logging, secret/content exclusion, loopback-only
  forwarded-address trust, handled error correlation, authentication failure,
  safe recovered-panic diagnostics, stock request-dump suppression, and CORS
  request-ID exposure have focused test coverage.
- DEBUG request-start and handled-error diagnostics have focused mode,
  correlation, classification, and secret/content leak coverage.
- Known CPO unique-constraint mappings and the unknown-constraint fallback have
  focused unit coverage; PostgreSQL lifecycle assertions now require the exact
  slug and GSTIN conflict codes.
- Required-field validation, slug normalization/authorization, migration
  content, and affected package tests passed for the source-tree change.
- The 70-operation source OpenAPI and runtime route sets match; documentation
  contract verification passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- Revision `d27e599` was built cleanly and rehosted without a migration. The
  installed identity, loopback-only listener, loopback/public readiness, live
  70-operation Swagger/OpenAPI, zero-restart service state, DEBUG request-start
  and completion records, and absence of startup errors or panics passed.
- Revision `9760523` was built cleanly and rehosted with migration eleven
  already current. The installed hash, loopback/public readiness, live
  70-operation Swagger/OpenAPI with field-specific conflict codes, protected
  and retired routes, required workers, and journal passed.
- Revision `afd90f5` was built cleanly, migrated through version eleven, and
  rehosted. The installed hash, loopback/public readiness, live 70-operation
  Swagger/OpenAPI, protected slug route, retired routes, required workers,
  migration constraints, and journal passed.
- Revision `1cec3f3` was built cleanly and rehosted. The installed hash,
  loopback/public liveness and readiness, live Swagger/OpenAPI, protected and
  retired routes, required workers, migration ledger, and journal passed.
- The read-only CPO organization projection, privileged-field omission,
  protected route, 69-operation OpenAPI parity, documentation contract, and
  complete CPO organization/profile/network/pricing lifecycle passed. The
  lifecycle used an explicitly created and removed disposable PostgreSQL
  database.
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
  revocation, and password reset handling passed. Updated lifecycle coverage
  obtains both recovery ID and code from the encrypted recipient mail payload
  rather than internal challenge storage; that changed PostgreSQL lifecycle was
  not run in this slice because no disposable `TEST_DATABASE_URL` was set.
- The mail worker claimed, decrypted, delivered through a test sender, and
  completed a durable job.
- Hostinger implicit-TLS configuration loaded successfully and the SMTP sender
  construction tests passed without exposing a real mailbox password.
- The required documentation, administrative route coverage, and removed SMTP
  configuration residue checks passed.
- OpenAPI parsing, semantic validation, all 40 Gin/OpenAPI operation matches,
  and docs/raw-spec HTTP smoke tests passed.
- Documentation routes were absent while ordinary health routes remained
  available with `API_DOCS_ENABLED=false`.
- Razorpay secrets remained encrypted in storage, resolved only internally for
  the correct CPO, and were unavailable to a platform principal.
- Route registration, unauthenticated rejection, request validation, and
  no-store behavior passed.
- The CPO provisioning migration rolled down, up, and idempotently up in
  PostgreSQL 17.
- CPO creation without commercial prerequisites, encrypted temporary-password
  delivery,
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
  password reset handling, password change, and global session revocation have
  PostgreSQL lifecycle coverage. The updated recipient-visible recovery path
  compiled but was not executed against PostgreSQL in this slice because no
  disposable `TEST_DATABASE_URL` was set.
- Permissive CORS preflight behavior and the disabled-CORS path passed focused
  route tests; authentication and authorization remain active in either mode.
- Platform operations compile; model parsing, migration discovery/pairing, mail
  rendering, route protection, and the retained realtime/worker contracts pass.
- The current 49 Gin/OpenAPI operations match bidirectionally; all retired
  commercial routes return `404` and are absent from OpenAPI.
- Migration ten applied and rolled back against disposable loopback PostgreSQL
  17. Its PostgreSQL CPO lifecycle test passed creation correlation,
  primary-admin uniqueness, search/cursor listing, profile replacement,
  reasoned idempotent activation, previous-primary session/refresh revocation,
  credential-free onboarding resend, and platform-session isolation.
- Migration nine applied against disposable loopback PostgreSQL 17, archived
  commercial tables without losing a preserved row, blocked pending commercial
  mail, disabled retired worker records, rolled down with data intact, and
  restored worker requirements.
- PostgreSQL execution of migrations six through eight passed against the
  PostgreSQL 18.4 development deployment before the commercial surface was
  retired.
- All ten forward migrations are recorded in the active deployment's
  `schema_migrations`; the runtime `public` schema contains 31 tables and
  `retired_commercial` contains the 11 archived prototype tables.
- Migration ten added CPO lifecycle evidence, primary-administrator
  designation, and safe mail-correlation fields without changing existing row
  counts; the pre-migration database contained no CPO or membership rows.
- The configured platform superadmin remains bootstrapped exactly once.
- Loopback and public HTTPS liveness and database-readiness checks passed.
- Swagger UI and raw OpenAPI return `200`; the live OpenAPI contains all 49
  operations, while unauthenticated requests to the platform-managed CPO
  profile and primary-administrator APIs return `401`.
- Migration nine marked subscription lifecycle and billing maintenance workers
  disabled and non-required. Platform maintenance and mail outbox remain the
  required runtime workers.
- Readiness is evaluated per required worker name and succeeds when at least
  one instance is fresh and healthy. Stale records from replaced processes no
  longer degrade a healthy replacement.
- A real platform-login request returned `202`, and its encrypted
  `LOGIN_OTP` outbox job reached `SENT` on the first attempt through the
  configured Hostinger SMTP account.

## Current Access Model

- `users` represent login identities.
- `platform_admins` explicitly grant platform-superadmin authority.
- `cpos` represent tenant/customer organizations.
- `cpo_memberships` store a fixed role inside one CPO; current callable
  authority requires `ADMIN`.
- `customers` represent a user's customer relationship with one CPO.

The full mapping from the supplied schema is recorded in `docs/SCHEMA.md`.

The same identity may belong to multiple CPOs. Its membership and customer
relationships remain distinct and tenant-scoped. Only ADMIN membership is
currently accepted for CPO sessions; other stored role values are dormant.

An administrative session selects exactly one platform or CPO scope. Protected
requests revalidate the durable session and current authority. Tenant context
comes from that principal rather than a request header. Tenant business routes
also verify that `X-CPO-App-ID` equals the current dummy or live ID for that
same principal; the app ID never grants authority or changes scope.

## HAL Boundary

The HAL remains a separate service and database. It is not embedded in this
repository. The integration contract has not been implemented yet.

## Known Limitations

- Password-recovery emails queued before recovery-ID delivery was implemented
  contain only the OTP and expiry and cannot complete reset. Users must request
  a new email; no database challenge lookup is an approved client workflow.
- A successful CPO-creation response proves its encrypted onboarding job
  committed, not that SMTP sent it. Operators must use primary-admin delivery
  status; only a newly created global identity receives a temporary password.
- Only the initial administrator profile and network/GST/tariff subset has
  handlers. Customer directory, access tokens, charging, wallets, payments,
  reporting, and most other domain tables remain without business APIs.
- CPO staff invitation after the first admin and customer email/profile-change
  workflows are not implemented.
- Tenant subscriptions, platform invoices, and platform payments are
  intentionally unsupported; CPO access is manual.
- Automatic encryption-key rotation is not implemented; data must be
  re-encrypted before an old key is removed.
- SMTP delivery logic is implemented, worker-tested, and verified through one
  real Hostinger platform-login OTP delivery. The mailbox password remains only
  in the ignored deployment environment.
- No generated frontend SDK exists yet; consumers use the reviewed OpenAPI
  contract directly.
- Migration eleven's disposable PostgreSQL lifecycle coverage has not executed
  because deleting a test database was not authorized. The live development
  deployment is current on migration eleven and the 70-operation contract.
