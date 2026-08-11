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
- ADMIN-only callable CPO authority initially, with dormant fixed-role enum
  capacity for a later staff-management design
- Loopback-only local development listener by default

## Permanent Engineering Invariants

- A CPO is a tenant organization, not a user role.
- Administrative login identity is global; app-customer identity, credentials,
  profile, and sessions are local to one CPO.
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
activate, suspend, and recover CPOs while the first CPO ADMIN manages the
initial tenant surface. Staff management remains a later explicit feature.

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

## Current Execution

Current phase:

- Phases 5 and 6: first CMS HAL charging vertical.

Active work:

- `docs/work/active/WI-20260811-cms-hal-first-charging-vertical.md`.

Current implementation state:

- CMS source and the development deployment contain the first client, durable
  records, fact receiver, customer polling/start/stop routes, and 157-operation
  OpenAPI surface. The HAL v1 provider is not configured on this host, and the
  complete Postgres-to-HAL-to-virtual-charger vertical is not verified yet.

Next required slice:

1. Add disposable PostgreSQL lifecycle tests and a bounded reconciliation
   worker for pending/ambiguous CMS commands.
2. Run the real loopback HAL and virtual OCPP charger acceptance topology,
   including duplicate, restart, and outage cases.

## Feature Registry

### Feature: Lean tenancy and access foundation

Status: Verified

Phase: Foundation

Objective:

Represent the minimum safe access model: platform superadmins, CPO tenant
organizations, fixed CPO-wide staff roles, and tenant-scoped customers.

Scope:

- Global administrative login identities
- Platform-superadmin marker
- CPO organization and lifecycle status
- Fixed CPO membership roles: owner, admin, operator, viewer
- Tenant-local customer account capacity (superseded by migration twenty)
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
- A customer account is scoped to a specific CPO.
- Duplicate memberships and normalized customer emails within one CPO are
  rejected by PostgreSQL.
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

Status: Implemented

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
- Eligible recovery mail contains the opaque challenge ID, code, and expiry
  required by reset while the forgot response remains generic.
- Tenant context comes from the verified session, not a client-supplied CPO
  header.
- CPO integration secrets are never returned by an API or stored as plaintext.
- Mail delivery survives process failure and can retry without logging OTPs.

Current verification limitation:

- Administrative and customer recovery emails now carry the opaque recovery
  ID, code, and expiry required by reset. OTP producers now call the canonical
  mail enqueue operation with the complete payload; the lossy OTP-only wrapper
  that dropped `challenge_id` no longer exists. Database-free validation,
  rendering, and full-suite checks pass, and lifecycle coverage consumes
  recipient-visible inputs rather than querying challenge storage. The changed
  PostgreSQL lifecycle has not run in this slice because no explicitly
  disposable `TEST_DATABASE_URL` is configured, so the feature remains
  `Implemented` rather than `Verified`.

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

### Feature: Manual CPO provisioning and app identity

Status: Implemented

Phase: Authentication and CPO administration

Depends on:

- Authentication and credential boundary

Enables:

- Future CPO staff invitation and membership management after an explicit
  capability design
- Trusted routing for every tenant business API
- Separate onboarding and production application identities

Objective:

Allow platform superadmins to create and control CPO tenants through explicit
manual lifecycle decisions, assign an opaque dummy application ID at creation,
replace it with a live application ID, and require the current ID on
authenticated CPO-scoped business requests.

Scope:

- Create, list, and inspect CPO tenants through platform-only APIs
- Require GSTIN and complete address fields for creation/profile replacement
- Enforce normalized global uniqueness for slug and GSTIN
- Expose an authenticated advisory slug-availability query for FE validation
- Report the exact known slug, GSTIN, app-ID, identity, or administrator
  membership uniqueness cause through stable `409` error codes
- Create or attach the first CPO admin identity and membership transactionally
- Encrypted email job and SMTP delivery of a generated temporary password for a
  new identity, with fail-closed payload validation
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

- Tenant subscriptions, platform invoices/payments, or feature matrices
- CPO self-provisioning
- General CPO owner/staff invitation after the first admin
- App ID as an authentication secret
- HAL or payment-provider callback authentication

Acceptance criteria:

- A CPO is created without any commercial access record or payment check.
- CPO, first admin identity/membership, audit records, and onboarding mail are
  committed atomically.
- A new first admin's committed welcome job contains a generated temporary
  password whose plaintext exists only in the encrypted payload, worker/SMTP
  renderer memory, and recipient email. Missing credential data fails the
  transaction.
- An existing identity is attached without changing its password.
- A new first admin must change the temporary password before tenant business
  APIs are allowed; login reminders continue without a password-expiry timeout.
- Creation always assigns a unique dummy app ID and a pending lifecycle status.
- Creation rejects missing/blank GSTIN, address, city, state, or pincode, and
  PostgreSQL preserves the same invariant outside the HTTP boundary.
- Slug availability returns the normalized candidate and current availability,
  while final creation still resolves concurrent uniqueness races.
- Known uniqueness races return a field- or relationship-specific conflict code
  instead of collapsing into an ambiguous CPO conflict.
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

Current verification limitation:

- The migration-eleven and PostgreSQL CPO lifecycle additions compile but have
  not executed because no explicitly disposable `TEST_DATABASE_URL` is set.
  The strengthened registration contract remains `Implemented` until that
  database verification passes.

Detailed plan:

- `docs/plans/cpo-provisioning-and-app-identity.md`

Architecture decision:

- `docs/decisions/0010-required-cpo-registration-identity.md`

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

### Feature: Safe HTTP request observability

Status: Verified

Phase: Cross-cutting operations

Depends on:

- HTTP route and recovery wiring
- Administrative and customer principal middleware

Objective:

Give operators one safe structured completion record for every request that
reaches Gin and give clients a response correlation ID without copying request
content or credentials into logs.

Scope:

- Always-on newline-delimited JSON logging to stdout
- Server-generated `X-Request-ID` on every Gin response and CORS exposure
- Matched route template, status, latency, response size, and safe client/peer
  address fields
- Trusted authenticated scope/user/CPO/customer/role enrichment
- Stable handled API `error_code` enrichment
- Logging outside panic recovery so recovered `500` requests are recorded
- Correlated panic stack diagnostics without Gin request dumps or panic values
- Explicit log-schema, proxy-trust, and data-exclusion contract
- Developer field-selection, severity, correlation, and no-duplicate-access-log
  guidance
- `LOG_LEVEL=DEBUG` request-start and handled-error component/type diagnostics
  under the same request ID

Non-goals:

- Request/response body, raw path, query, header, email, user-agent, app-ID,
  credential, token, OTP, API-message, database-error, or panic-value logging
- A log database, shipping agent, dashboard, metrics system, or distributed
  tracing
- Application-owned retention or rotation
- A debug mode that logs payloads, raw URLs/queries, header values, secrets,
  personal fields, error strings, SQL, or provider content

Acceptance criteria:

- Every Gin response carries a UUID request ID and emits one completion record.
- Handled errors expose their stable code in the record without their message.
- Authenticated records use only trusted server-established identifiers.
- Direct clients cannot spoof `client_ip` with forwarding headers; only a
  loopback proxy is trusted.
- Recovered panics produce a correlated safe stack diagnostic and an
  `ERROR`/`500` completion record without request content or the panic value.
- Focused leak tests prove bodies, query values, raw path identifiers,
  authorization values, and user agents are absent.
- DEBUG mode adds start/error-classification events without changing any data
  exclusion.

Verification:

- `go test ./src/config ./src/middleware ./src/routes -run 'TestConfig|TestLoadHostinger|TestRequestLog|TestRequestLogger|TestDebugLogging|TestPermissiveCORS' -count=1`
- `go test ./...`
- `go vet ./...`
- `.\scripts\verify-docs.ps1`
- `git diff --check`

Contract:

- `docs/contracts/internal/http-request-logging.md`

Architecture decision:

- `docs/decisions/0011-safe-http-request-observability.md`

### Feature: CPO-scoped customer signup

Status: Verified

Phase: Phase 4: Customers, access tokens, and tariffs

Depends on:

- Authentication and credential boundary
- Manual CPO provisioning and app identity

Objective:

Allow a customer to verify their email and create a tenant-scoped customer
account and wallet through an active CPO application.

Scope:

- Public start, verify, and resend endpoints
- Active-CPO resolution through current `X-CPO-App-ID`
- Durable OTP and encrypted mail delivery
- Transactional CPO-local customer account and INR wallet creation

Non-goals:

- Customer login or session issuance
- Commercial access or payment checks
- Social login, SMS OTP, or app attestation

Acceptance criteria:

- Verification creates exactly one tenant customer and wallet.
- Same-email accounts under different CPOs retain independent credentials and
  profiles.
- OTP, rate-limit, replay, concurrency, CPO lifecycle, and tenant boundaries
  are enforced durably.

Detailed plan:

- `docs/plans/customer-signup.md`

### Feature: Complete app-user authentication boundary

Status: Implemented; PostgreSQL lifecycle verification pending

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
- Trusted customer/CPO/app-ID handler helpers and a source-compatible customer
  ID alias for older app modules

Non-goals:

- Social login, SMS, TOTP, passkeys, or device attestation
- Customer profile editing (next slice)
- Staff/customer impersonation

Current verification limitation:

- Customer recovery email now delivers the recovery ID, code, and expiry while
  the forgot response remains generic. Unit/rendering/full-suite checks pass,
  but the changed PostgreSQL lifecycle has not run without an explicitly
  disposable `TEST_DATABASE_URL`, so this feature remains `Implemented`.

Detailed plan:

- `docs/plans/customer-authentication.md`

### Feature: Customer app experience

Status: In Progress — profile, published-network, favorites, and informational tariff slices implemented; PostgreSQL lifecycle verification deferred by decision

Phase: Phase 4, then Phases 5 and 6 for HAL-dependent work

Depends on:

- Complete app-user authentication boundary
- Initial CPO network and tariff configuration

Enables:

- Customer self-service profile and station discovery
- Safe customer favorites and server-calculated price display
- A later CMS/HAL customer charging lifecycle
- Wallet, billing, session-history, receipt, notification, and realtime work

Objective:

Build the customer-facing CMS API in coherent dependency order with separate
CPO-scoped customer accounts, while keeping global administrative identities
and the HAL boundary intact.

Initial implementation slice:

- Replace global customer identity reuse with CPO-scoped customer accounts,
  then refactor profile editing to use the existing `X-CPO-App-ID` contract.

Non-goals:

- Changes to CPO ADMIN route ownership or its existing app-ID header
- Live charger availability, remote charging commands, HAL transport, wallet
  mutations, payment execution, or realtime before their durable foundations

Detailed plan:

- `docs/plans/customer-app-experience.md`

Architecture decision:

- `docs/decisions/0013-cpo-scoped-customer-accounts.md`

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

Status: Implemented

Phase: Phase 2: Authentication and CPO administration

Depends on:

- Authentication and credential boundary
- Manual CPO provisioning and app identity
- Durable encrypted mail outbox

Enables:

- Complete platform operations without tenant-data bypass
- Recoverable mail, notifications, and worker operations
- Reconnect-safe platform realtime
- Operational superadmin frontend

Objective:

Finish the platform-management plane across manual CPO lifecycle, platform
administrators, audit/security, mail/notifications, workers, announcements,
overview, status, and durable realtime delivery.

Current integration contract:

- `docs/SUPERADMIN_FRONTEND_HANDOFF.md` is the canonical no-chat-history FE
  guide and keeps implemented, blocked, planned, and intentionally unsupported
  behavior separate.

Current source implementation:

- Migration fourteen adds platform-authority status fields and durable
  announcement/notification records; migration fifteen adds tariff effective
  dates and database-enforced overlap protection.
- Governance, security, mail, announcement/notification, overview, and status
  routes are implemented and represented with the CPO user lookup in the
  124-operation source OpenAPI contract. The added CPO charger hub-assignment
  operation is CPO ADMIN-only and does not extend SuperAdmin authority.
- Focused source tests, route/OpenAPI parity, documentation verification, the
  full Go suite, vet, and diff checks pass.

Verification limitation:

- Disposable PostgreSQL lifecycle execution remains pending because no
  `TEST_DATABASE_URL` is configured. The source implementation is not marked
  `Verified` until migration fourteen and the stateful governance/mail/
  notification flows are exercised against an explicitly disposable database.

Non-goals:

- Superadmin access to tenant business data or tenant secret plaintext
- Platform invoices, platform payments, checkout, provider webhooks, or
  automatic commercial lifecycle
- Redis, NATS, or a separate realtime service

Detailed plan:

- `docs/plans/superadmin-control-plane.md`

Architecture decision:

- `docs/decisions/0007-complete-superadmin-control-plane.md`, as revised by
  `docs/decisions/0008-manual-cpo-access-without-commercial-management.md`

### Feature: Manual platform subscriptions

Status: In Progress

Phase: Phase 2: Authentication and CPO administration

Depends on:

- Platform-superadmin authentication and audit/event operations
- Existing migration-nine retired commercial preservation boundary

Enables:

- Manual plan catalog and immutable published version management
- Manual CPO subscription issue, renewal, state control, and history without
  provider infrastructure

Objective:

Restore the subscription infrastructure as an explicit superadmin-operated
system while retaining the retirement of platform billing and all automatic
provider/lifecycle behavior.

Non-goals:

- CPO access control based on subscription status or dates
- Invoice, payment, checkout, collection, webhook, or provider integration
- Scheduled plan changes, period-end cancellation, automatic renewal, trial
  completion, expiry, or subscription email
- Feature keys, entitlement overrides, or feature enforcement before a module
  catalog and server-side gates are approved

Detailed plan:

- `docs/plans/manual-platform-subscriptions.md`

Architecture decision:

- `docs/decisions/0012-manual-platform-subscriptions-without-provider.md`

### Feature: CPO administrator and initial network configuration

Status: Verified

Phase: Phases 2, 3, and the GST/tariff foundation of Phase 4

Depends on:

- Authentication and credential boundary
- Manual CPO provisioning and app identity
- Complete CMS schema baseline
- Platform CPO dependency surface

Enables:

- First CPO frontend development against one unambiguous administrator persona
- Future authenticated CMS/HAL charger-mapping handshake
- Later customer-directory and pricing consumption
- Deliberate future staff-management expansion without changing tenant keys

Objective:

Reconcile the contributed CPO implementation into a correct ADMIN-only surface
for administrator identity profile, a read-only tenant organization projection,
hubs, chargers/connectors, GST, and tariffs without adding tenant-side
organization mutation or implying HAL integration.

Scope:

- ADMIN-only CPO authentication and authorization
- CPO administrator identity profile read/update
- Session-bound, read-only CPO organization details
- Tenant-scoped network and pricing create/read/update operations
- Bounded charger listing and dependency-safe charger deletion
- Server-generated identifiers and exact decimal pricing/tax values
- Transactional audit evidence
- OpenAPI, human contract, educational/integration guidance, and verification

Non-goals:

- App-user/customer changes
- Staff invitation, role management, or callable non-ADMIN roles
- Tenant-side CPO organization mutation
- Complete CRUD for every table
- HAL handshake, live charger status, commands, callbacks, or tenant realtime

Detailed plan:

- `docs/plans/cpo-admin-network-configuration.md`

Agent handoff:

- `docs/CPO_BACKEND_AGENT_HANDOFF.md`

Architecture decision:

- `docs/decisions/0009-admin-only-cpo-authority.md`

### Feature: CPO customer directory

Status: In Progress

Phase: Follow-up to CPO administration

Depends on:

- CPO administrator and initial network configuration
- CPO-scoped customer signup

Enables:

- CPO visibility into their registered user base.
- Future CPO-side customer support and management workflows.

Objective:

Provide CPO administrators with a read-only view of their registered app
customers, scoped to their own tenant.

Scope:

- Authenticated CPO ADMIN endpoints for listing and viewing customers.
- Keyset pagination and basic search/filter capabilities.
- Safe customer data projection, excluding credentials and sensitive information.

Non-goals:

- CPO-side customer creation, mutation, or deletion.
- Customer group or RFID management.

## Current Execution

Current phase:

- Phase 2: Authentication and CPO administration, with initial Phase 3/4 CPO
  configuration implemented

Active feature:

- Customer app experience — User App Razorpay wallet recharge

Current implementation slice:

- Separated User App credential/session routes at `/api/v1/app/auth` from
  authenticated app-resource routes at `/api/v1/app`; handlers, OpenAPI,
  verification expectations, and frontend/integration guidance move together.
- Added User App Razorpay order creation and checkout verification using the
  existing encrypted CPO integration credentials. Durable recharge orders,
  provider payment attempts, future-refund records, provider snapshots, and
  the atomic wallet-credit ledger link are now in migration 22. No CPO or
  Superadmin payment API, RFID flow, webhook, refund command, or HAL call is
  added.
  Disposable PostgreSQL lifecycle checks remain deferred by decision until
  explicitly reactivated.

Last completed slice:

- Added tenant-scoped user-group member assignment with same-group idempotency,
  cross-group conflict handling, audit evidence, OpenAPI parity, and protected
  route verification. This source revision was rehosted without a new
  migration.
- Added tenant-scoped CPO hub, charger, and user-group tariff operations plus
  protected user-group CRUD, with service validation, OpenAPI parity, and
  integration coverage. This source revision was rehosted without a new
  migration.
- Split User App hub and charger opening-hour fields into distinct serialized
  contract values (`open_24_hours`, `twenty_four_seven_open_status`, and
  `hub_open_24_hours`), with opposing-value projection coverage and synchronized
  OpenAPI/frontend guidance. This source revision was rehosted without a new
  migration.
- Propagated display-safe charger metadata (`charger_name`, category/use,
  parking, and related fields) through the authenticated User App discovery,
  detail, hub, and favorite projections, with the OpenAPI and frontend handoff
  updated. This source revision was rehosted without a new migration.
- Implemented User App Razorpay wallet recharge order creation and captured
  payment verification with SDK-backed provider calls, signature and exact
  order/payment matching, idempotent wallet credit, encrypted CPO credential
  resolution, and durable provider order/payment/refund records in migration
  22. Refund execution, webhooks, settlement reconciliation, RFID, and HAL
  remain separate follow-ups.
- Implemented User App charger search/filter and bounded near-me results over
  published hubs, with safe hub/connector projections, DB-backed administrative
  status and connector total capacity, an authenticated charger-image route
  keyed by public charger ID, and explicit UNKNOWN live availability;
  implemented wallet balance and keyset-paginated wallet-history reads.
  Recharge, refund, charging-session history, RFID, and HAL remain separate
  slices.
- Implemented the effective customer tariff resolver and informational hub and
  charger price APIs with exact decimal projections, explicit unavailable state,
  active GST resolution, and User Tariff > charger tariff > hub tariff
  precedence.
- Implemented customer favorites over the published discovery projection,
  including bounded independent cursors, idempotent mutations, audit actions,
  unpublish-safe reads, route/OpenAPI parity, and User App documentation.
- Implemented published customer network discovery and the CPO hub publication
  switch, including migration 21, safe projections, route/OpenAPI parity, and
  User App/CPO documentation.
- Implemented customer self-service profile editing on the CPO-local customer
  account with strict fields, omitted-versus-null phone semantics, transactional
  audit evidence, and the canonical `UserView` response.
- Reconciled and deployed the tenant-scoped CPO user point lookup plus tariff
  effective dates, with mandatory hub scope and migration-fifteen exclusion
  enforcement
- The previous recovery-payload release and its hosted verification are
  complete; manual subscriptions now supersede the retired-subscription
  non-goal under ADR 0012.
- Removed the lossy OTP-only outbox wrapper so administrative and customer
  recovery enqueue the complete canonical payload with `challenge_id`; added
  database-free validation for both reset templates. Revision `d0059fe` was
  deployed without sending a live recovery email for release verification.
- Implemented safe JSON HTTP completion logging, server-generated response
  request IDs, trusted auth/error-code enrichment, loopback-only forwarding
  trust, CORS exposure, safe DEBUG lifecycle/error classification, and a strict
  content/secret exclusion contract
- Added constraint-aware `409` errors for CPO slug, GSTIN, app ID,
  administrator identity, membership, and primary-administrator collisions,
  retaining `cpo_conflict` only as an unknown-constraint fallback
- Implemented mandatory GSTIN/address registration and profile invariants,
  preserved database-authoritative normalized slug/GSTIN uniqueness, and added
  the authenticated advisory slug-availability contract; PostgreSQL execution
  remains pending a disposable `TEST_DATABASE_URL`
- Implemented enumeration-safe recovery-ID delivery for administrative and
  customer reset mail; made credential-bearing reset/welcome payloads fail
  closed; verified reset and first-admin SMTP rendering; and clarified
  new-versus-existing identity plus queued-versus-sent onboarding behavior
- Added the canonical exhaustive SuperAdmin frontend integration handoff and
  corrected current event examples/payload guidance plus the administrative
  password-recovery readiness claim
- Reconciled the contributed CPO branch into an ADMIN-only identity,
  read-only organization, hub/charger/connector, GST, and tariff surface with
  69-operation runtime/OpenAPI parity
- Verified the complete CPO dependency on Superadmin: searchable/paginated CPO
  administration, mutable business profile, reasoned lifecycle control,
  primary-admin recovery/onboarding resend, and CPO administrative-session
  revocation
- Implemented the remaining platform governance, security, mail, notification,
  announcement, overview, and status source surfaces; focused tests and full
  contract/docs verification are the active follow-up.

Last deployment milestone:

- Revision `e5fd599` was built from a clean worktree and rehosted on the
  development VPS after applying migration thirty. The tenant-scoped CPO
  settings API and its aligned multipart OpenAPI contract are live alongside
  the nullable tariff metadata, SuperAdmin administrator-list binding, and
  repaired `UserGroup.members` schema. Running-binary SHA/VCS identity,
  active/enabled systemd state, loopback/public liveness and readiness,
  Swagger, the live 160-operation OpenAPI surface, protected routes, and the
  migration-created settings table were verified. The bounded SSE shutdown
  deadline occurred during stop and recovered through systemd; current state is
  healthy with zero restarts.
  The disposable PostgreSQL lifecycle and full HAL/virtual-charger acceptance
  remain pending without the required test topology and `TEST_DATABASE_URL`.

Next expected slice:

- Add supported customer charging-session history projections from durable CMS
  session records, without inventing HAL control or live availability.

Blocked by:

- None

## Next Approved Work

1. Add supported customer charging-session history projections from durable CMS
  session records, without inventing HAL control or live availability.
2. Design the CMS/HAL QR-scan charging lifecycle before adding start/stop APIs.

Deferred verification decision:

- Disposable PostgreSQL lifecycle tests are intentionally skipped for the
  foreseeable workstream. Do not treat their absence as a feature blocker or
  claim stateful verification; preserve the tests and reactivate them only by
  an explicit decision.

## Deferred Work

- Custom tenant roles and fine-grained permissions
- Per-hub staff scopes
- PostgreSQL row-level security
- Cross-CPO roaming
- Redis, NATS, and service decomposition

## Risks and Unresolved Decisions

- Reset emails queued before recovery-ID delivery was implemented cannot be
  completed and must be replaced by a fresh request after the corrected backend
  is deployed.
- CPO creation commits an encrypted welcome job but cannot guarantee later SMTP
  delivery; the SuperAdmin FE must use the primary-admin delivery status and
  reserve “sent” for `SENT`.
- Key rotation will initially require an explicit re-encryption operation before
  removing an old encryption key; automatic key rotation is deferred until a
  concrete operational requirement exists.
- SMTP availability is an operational dependency for administrative login and
  password recovery.
- CPO access is an explicit platform-superadmin activation/suspension decision;
  manual subscription records never control tenant authorization.
- HAL v1 is consumed through `integrations/ocpp-hal-boundary.md`; do not extend
  the provider contract without a separate approved contract change.
- `OWNER`, `OPERATOR`, and `VIEWER` are dormant schema capacity only. Their
  authorization semantics require a future approved staff-management plan.

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

The historical fifth migration was verified previously. Migration twenty now
separates customer credentials and auth lineage from administrative users; its
static migration contract and database-free auth tests are covered, while its
full PostgreSQL lifecycle remains pending an explicit `TEST_DATABASE_URL`.

Migration six and platform operations have focused
compile/model/migration-discovery/route/OpenAPI tests. Migrations seven and
eight are retained as immutable deployment history only. Migration nine
retires their tables into `retired_commercial`; its disposable PostgreSQL 17
up/guard/data-preservation/worker-disable/down lifecycle passed.
Migration twelve restored six subscription tables with immutable published plan
snapshots. Migration thirteen returns the two dormant entitlement tables to
`retired_commercial`; its manual lifecycle test requires a disposable
`TEST_DATABASE_URL`. Platform-billing tables and automated workers remain
retired.

## Completion Criteria

The CMS is complete only when tenant boundaries, network management, HAL
integration, charging recovery, exact billing, operational verification, and
current project documentation work together end to end.
