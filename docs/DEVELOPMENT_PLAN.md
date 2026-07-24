# Development Plan

## Project Objective

Build a maintainable CMS that TransEV can sell to CPO organizations so each CPO
can manage its staff, customers, charging network, pricing, charging sessions,
and financial operations without accessing another CPO's data.

## System Boundaries

- The CMS owns platform administration, CPO tenancy, people and access,
  commercial network data, tariffs, customer accounts, charging-session
  projections, billing, and reporting.
- The separate Go HAL owns OCPP connections, charger protocol state, exact OCPP
  transaction identifiers, raw meter communication, command delivery, and
  protocol recovery.
- The services integrate through authenticated and idempotent APIs. They do not
  share database ownership.

## Architectural Direction

- Go modular monolith
- PostgreSQL as durable source of CMS truth
- Gin HTTP API
- GORM for application persistence
- Explicit versioned SQL migrations
- Fixed CPO-wide staff roles initially
- Loopback-only local development listener by default

## Permanent Engineering Invariants

- A CPO is a tenant organization, not a user role.
- Login identity is global; CPO staff membership and CPO customer membership are
  tenant-scoped.
- Platform superadmin authority is separate from CPO membership.
- CPO suspension blocks new tenant operations but must never prevent completion,
  stopping, callback ingestion, or billing of an already active session.
- Money will use an exact representation and energy used for billing will use
  integer Wh.
- The CMS never invents or replaces a HAL-issued OCPP transaction identifier.

## Development Phases

### Phase 1: Foundation

Objective: establish a compilable service, durable migrations, CPO tenancy,
identity, fixed staff roles, customer membership, the complete supplied CMS
domain schema, and project verification.

Completion criteria:

- Service starts with an explicit database connection.
- Versioned migrations are applied once and recorded.
- Platform admins, CPOs, CPO memberships, and customers have enforced database
  relationships.
- Settings, groups, hubs, chargers, connectors, tariffs, GST, favorites,
  wallets, transactions, sessions, payments, and audit logs are represented.
- Health endpoints distinguish process health from database readiness.
- Focused and repository-wide Go checks pass.

### Phase 2: Authentication and CPO administration

Depends on Phase 1.

Objective: authenticate users and allow a platform superadmin to provision,
activate, and suspend CPOs while CPO owners manage their own staff.

### Phase 3: Charging network inventory

Depends on Phase 2.

Objective: manage tenant-owned hubs, chargers, and connectors and define the
minimal charger-directory contract with the separate HAL.

### Phase 4: Customers, access tokens, and tariffs

Depends on Phase 3.

Objective: manage CPO customers, RFID/idTags, customer groups, immutable tariff
versions, and tax rules.

### Phase 5: Charging lifecycle and HAL integration

Depends on Phases 3 and 4.

Objective: implement durable remote-command intents, idempotent HAL callbacks,
exact transaction-ID preservation, session recovery, and reconciliation.

### Phase 6: Wallet and billing

Depends on Phase 5.

Objective: calculate immutable session charges and settle them through an
atomic, idempotent wallet ledger.

## Feature Registry

### Feature: Lean tenancy and access foundation

Status: Verified

Phase: Foundation

Objective:

Represent the minimum safe access model: platform superadmins, CPO tenant
organizations, fixed CPO-wide staff roles, and tenant-scoped customers.

Scope:

- Global login identities
- Platform-superadmin marker
- CPO organization and lifecycle status
- Fixed CPO membership roles: owner, admin, operator, viewer
- Tenant customer relationship
- Versioned initial migration
- Health and database-readiness endpoints

Non-goals:

- Custom roles or permissions
- Hub-level staff scopes
- Subscription plans and feature entitlements
- Authentication endpoints
- Network inventory
- Charging, HAL callbacks, wallets, payments, or reporting

Acceptance criteria:

- A user can belong to more than one CPO.
- A user's staff role is scoped to a specific CPO.
- A customer relationship is scoped to a specific CPO.
- Duplicate membership and customer relationships are rejected by PostgreSQL.
- Invalid lifecycle and role values are rejected by PostgreSQL.
- No tenant-owned data is represented without a CPO identifier.

Verification:

- Model and constants unit tests
- Embedded migration discovery test
- Health-route tests
- `go test ./...`
- `go vet ./...`
- `git diff --check`

### Feature: Complete CMS schema baseline

Status: Verified

Phase: Foundation

Depends on:

- Lean tenancy and access foundation

Objective:

Preserve every business data area in the supplied schema without restoring its
global CPO role or cross-tenant relationships.

Scope:

- User settings and tenant user groups
- Hubs, chargers, connectors, and group access links
- Customer favorite hubs and chargers
- GST profiles and tariffs
- Wallets and wallet transactions
- Charging sessions and payments
- Audit logs
- Matching up and down SQL migrations
- Explicit latest-migration rollback command

Non-goals:

- CRUD or workflow APIs for these tables
- HAL command and callback contracts
- Charging or billing orchestration
- Seed or production data migration from the old CMS

Acceptance criteria:

- Every supplied business model maps to an implemented table or an explicitly
  documented replacement.
- Tenant-owned relationships cannot cross CPO boundaries.
- Financial fields avoid floating-point storage.
- Up and down migrations cover the same table set.
- GORM can parse every model relationship.

Verification:

- Complete-domain migration coverage test
- Up/down pair test
- GORM schema parsing test
- `go test ./...`
- `go vet ./...`
- `git diff --check`

### Feature: Authentication and credential boundary

Status: Verified

Phase: Authentication and CPO administration

Depends on:

- Lean tenancy and access foundation

Enables:

- Superadmin CPO provisioning
- CPO staff administration
- Every later tenant-scoped API
- Payment-provider integration without exposing provider secrets

Objective:

Provide one secure authentication boundary for global identities, platform
superadmins, and CPO staff, including idempotent superadmin bootstrap, mandatory
email OTP for administrative login, encrypted access tokens, rotating sessions,
password recovery, authorization helpers, and encrypted tenant integration
credentials.

Scope:

- Environment-only, idempotent first-superadmin bootstrap
- Argon2id password hashing and bounded account lockout
- Platform and CPO administrative login scopes
- Durable email OTP challenges and SMTP outbox worker
- Signed-then-encrypted short-lived access tokens
- Opaque, hashed, one-time rotating refresh tokens
- Session listing, current/all/specific-session revocation, and password change
- Enumeration-safe password recovery
- Trusted principal, user, CPO, and role helpers for later APIs
- CPO-admin write-only Razorpay credentials encrypted at rest
- Privileged-operation audit records

Non-goals:

- Public signup, social login, SMS OTP, passkeys, or authenticator-app TOTP
- Custom roles, per-hub staff scope, or a general secret vault
- Payment execution or Razorpay API calls
- Redis, an external queue, or a separate identity service
- Platform-superadmin access to tenant secret plaintext

Acceptance criteria:

- Repeated startup does not duplicate or overwrite the configured superadmin.
- Administrative access requires password plus an unexpired, single-use email
  OTP.
- A protected request uses a fully validated encrypted token and an active
  durable session.
- Refresh-token rotation detects reuse and revokes the affected session.
- Password reset is enumeration-safe and revokes existing sessions on success.
- Tenant context comes from the verified session, not a client-supplied CPO
  header.
- CPO integration secrets are never returned by an API or stored as plaintext.
- Mail delivery survives process failure and can retry without logging OTPs.

Verification:

- Password, encryption, token, OTP, session, middleware, and credential tests
- Embedded migration discovery and up/down pairing tests
- PostgreSQL migration and repository integration verification
- Route-level authentication and authorization tests
- `go test ./...`
- `go vet ./...`
- `git diff --check`

Detailed plan:

- `docs/plans/authentication-and-credentials.md`

### Feature: Subscription-independent CPO provisioning and app identity

Status: Verified

Phase: Authentication and CPO administration

Depends on:

- Authentication and credential boundary

Enables:

- CPO owner staff invitation and membership management
- Trusted routing for every tenant business API
- Separate onboarding and production application identities

Objective:

Allow platform superadmins to create and control CPO tenants without requiring a
subscription, assign an opaque dummy application ID at creation, replace it
with a live application ID, and require the current ID on authenticated
CPO-scoped business requests.

Scope:

- Create, list, and inspect CPO tenants through platform-only APIs
- Create or attach the first CPO admin identity and membership transactionally
- Encrypted email delivery of a generated temporary password for a new identity
- Existing global identity reuse without password reset
- Durable first-login password-change requirement and login reminders
- Server-generated unique dummy app ID on every CPO
- Independent pending, active, and suspended CPO lifecycle
- Superadmin activation and suspension
- Superadmin replacement/rotation of the live app ID
- `X-CPO-App-ID` validation against the authenticated CPO
- Current app ID returned by CPO authentication bootstrap responses
- Audit records for lifecycle and app-ID changes without treating app IDs as
  credentials

Non-goals:

- Subscription plans, billing entitlements, or feature matrices
- CPO self-provisioning
- General CPO owner/staff invitation after the first admin
- App ID as an authentication secret
- HAL or payment-provider callback authentication

Acceptance criteria:

- A CPO is created without any subscription record or subscription check.
- CPO, first admin identity/membership, audit records, and onboarding mail are
  committed atomically.
- A new first admin receives a generated temporary password whose plaintext is
  present only in the encrypted mail job and worker memory.
- An existing identity is attached without changing its password.
- A new first admin must change the temporary password before tenant business
  APIs are allowed; login reminders continue without a password-expiry timeout.
- Creation always assigns a unique dummy app ID and a pending lifecycle status.
- Activation permits CPO administrative login while retaining the dummy app ID.
- Superadmin can replace the dummy/current app ID with a validated live ID.
- Only the current app ID is accepted on CPO-scoped business APIs.
- Platform and authentication endpoints do not require a CPO app ID.
- CPO login verification, refresh, and `me` expose the current app ID so a
  client can recover from rotation.
- App ID mismatch never changes the authenticated CPO context.

Verification:

- App-ID generation and validation tests
- Platform route and authorization tests
- Tenant app-ID middleware tests
- PostgreSQL provisioning, activation, suspension, and rotation integration
  tests
- Migration up/down verification
- `go test ./...`
- `go vet ./...`
- `git diff --check`

Detailed plan:

- `docs/plans/cpo-provisioning-and-app-identity.md`

### Feature: Documentation system, OpenAPI explorer, and Hostinger SMTP contract

Status: Verified

Phase: Phase 2: Authentication and CPO administration

Objective:

Make the implemented credential boundary operable without chat history and
configure the production mailbox through an explicit encrypted SMTP transport.

Scope:

- Hostinger implicit TLS on port 465
- Mutually exclusive implicit-TLS and mandatory-STARTTLS configuration
- Educational identity, onboarding, and troubleshooting guides
- SMTP, Razorpay-storage, and HAL-boundary integration records
- Consolidated HTTP, mail-outbox, and environment contracts
- Complete OpenAPI 3.1 schemas for every implemented endpoint
- Embedded same-origin Swagger UI and raw OpenAPI routes
- Bidirectional runtime/OpenAPI operation drift verification
- Repository documentation registration and automated residue checks

Acceptance criteria:

- Mail-enabled startup rejects plaintext, ambiguous, or unpaired credential
  configuration.
- The checked-in example contains only non-secret Hostinger values.
- Documentation separates implemented behavior from future HAL and payment
  orchestration.
- Required documentation and removed configuration names are automatically
  checked.
- `/docs/` permits interactive testing without a CDN and `/openapi.yaml`
  exposes the validated source contract.
- Runtime and OpenAPI method/path sets agree exactly.

Verification:

- `go test ./src/config ./src/mail -count=1`
- `go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1`
- `.\scripts\verify-docs.ps1`
- `go test ./...`
- `go vet ./...`
- `git diff --check`

### Feature: CPO-scoped customer signup

Status: Verified

Phase: Phase 4: Customers, access tokens, and tariffs

Depends on:

- Authentication and credential boundary
- Subscription-independent CPO provisioning and app identity

Objective:

Allow a customer to verify their email and create a tenant-scoped customer
account and wallet through an active CPO application.

Scope:

- Public start, verify, and resend endpoints
- Active-CPO resolution through current `X-CPO-App-ID`
- Durable OTP and encrypted mail delivery
- Safe global identity creation or reuse
- Transactional customer and INR wallet creation

Non-goals:

- Customer login or session issuance
- Subscription checks
- Social login, SMS OTP, or app attestation

Acceptance criteria:

- Verification creates exactly one tenant customer and wallet.
- Existing identities are attached without credential or profile overwrite.
- OTP, rate-limit, replay, concurrency, CPO lifecycle, and tenant boundaries
  are enforced durably.

Detailed plan:

- `docs/plans/customer-signup.md`

### Feature: Complete app-user authentication boundary

Status: Verified

Phase: Phase 4: Customers, access tokens, and tariffs

Depends on:

- CPO-scoped customer signup
- Authentication and credential boundary

Objective:

Give each active CPO customer a complete, isolated app session lifecycle and
trusted backend principal helpers.

Scope:

- Password plus mail-OTP login
- Signed/encrypted access tokens and rotating refresh tokens
- Customer `me`, session listing/revocation, and logout
- Customer-scoped password recovery and authenticated password change
- Trusted user/customer/CPO/app-ID handler helpers

Non-goals:

- Social login, SMS, TOTP, passkeys, or device attestation
- Customer profile editing
- Staff/customer impersonation

Detailed plan:

- `docs/plans/customer-authentication.md`

### Feature: Temporary unrestricted development listener

Status: Verified

Phase: Phase 2: Authentication and CPO administration

Objective:

Allow frontend developers on other machines to reach the CMS and use its APIs
from arbitrary browser origins during the current integration period.

Scope:

- All-interface IPv4 listener through `HTTP_ADDR=0.0.0.0:8080`
- Environment-controlled permissive origins and preflight headers
- Focused enabled/disabled CORS route tests
- Explicit production rollback guidance

Non-goals:

- Disabling authentication, authorization, app-ID, or tenant boundaries
- Production TLS termination or a permanent origin policy

### Feature: Complete superadmin control plane

Status: In Progress

Phase: Phase 2: Authentication and CPO administration

Depends on:

- Authentication and credential boundary
- Subscription-independent CPO provisioning and app identity
- Durable encrypted mail outbox

Enables:

- Complete platform operations without tenant-data bypass
- Provider-neutral CPO licensing and billing records
- Recoverable mail, notifications, and worker operations
- Reconnect-safe platform realtime
- Operational superadmin frontend

Objective:

Finish the platform-management plane across CPO lifecycle, subscriptions,
entitlements, billing records, platform administrators, audit/security,
mail/notifications, workers, announcements, overview, status, and durable
realtime delivery.

Non-goals:

- Superadmin access to tenant business data or tenant secret plaintext
- Automatic subscription payment collection before provider approval
- Redis, NATS, or a separate realtime service

Detailed plan:

- `docs/plans/superadmin-control-plane.md`

Architecture decision:

- `docs/decisions/0007-complete-superadmin-control-plane.md`

## Current Execution

Current phase:

- Phase 2: Authentication and CPO administration, with the approved customer
  signup slice from Phase 4 completed early

Active feature:

- Complete superadmin control plane

Current implementation slice:

- Complete CPO lifecycle/admin recovery and platform-superadmin governance

Last completed slice:

- Provider-neutral platform billing accounts, immutable invoices/lines,
  payments, allocation reversal, overdue worker, and complete contracts

Last deployment milestone:

- Revision `e15699d` was built, migrated through version eight, and rehosted on
  the development VPS. Public and loopback health, API documentation, protected
  route behavior, required worker heartbeats, running-binary identity, and
  error-free startup logs were verified.

Next expected slice:

- Mail/security operations followed by notifications and announcements

Blocked by:

- None

## Next Approved Work

1. Complete the superadmin control-plane slices in
   `docs/plans/superadmin-control-plane.md`.
2. Add CPO owner staff invitation and membership management.

## Deferred Work

- Custom tenant roles and fine-grained permissions
- Per-hub staff scopes
- PostgreSQL row-level security
- Cross-CPO roaming
- Redis, NATS, and service decomposition

## Risks and Unresolved Decisions

- Key rotation will initially require an explicit re-encryption operation before
  removing an old encryption key; automatic key rotation is deferred until a
  concrete operational requirement exists.
- SMTP availability is an operational dependency for administrative login and
  password recovery.
- Commercial subscription rules remain intentionally outside CPO creation and
  authorization; CPO activation is a manual platform action.
- The exact CMS/HAL API contract will be defined with the charging-network and
  charging-lifecycle phases.

## Verification Strategy

Run focused unit tests first, then `go test ./...`, `go vet ./...`,
`git diff --check`, and `git status --short`. Database integration tests will be
run with an explicitly selected disposable `TEST_DATABASE_URL`. The first five
migrations have been verified up, idempotently up again, and down against
disposable local PostgreSQL 17 databases. The credential boundary has
PostgreSQL integration coverage for bootstrap, platform/CPO login, refresh
reuse, recovery, mail outbox, and tenant integration secrets. The third
migration has been verified down, up, and idempotently up; its PostgreSQL
lifecycle test covers first-admin onboarding, password enforcement, activation,
app-ID rotation, identity reuse, and suspension. The fourth migration has been
verified down, up, and idempotently up; its lifecycle test covers customer
signup resend, replay rejection, identity creation/reuse, tenant isolation, and
wallet creation.

The fifth migration has been verified down, up, and idempotently up; its
lifecycle test covers customer login, access/refresh validation, `me`, scoped
session management, password recovery/change, and revocation.

Migrations six through eight, platform operations, subscriptions, and platform
billing have focused compile/model/migration-discovery/route/OpenAPI tests.
Their disposable PostgreSQL migration and lifecycle verification is still
pending.

## Completion Criteria

The CMS is complete only when tenant boundaries, network management, HAL
integration, charging recovery, exact billing, operational verification, and
current project documentation work together end to end.
