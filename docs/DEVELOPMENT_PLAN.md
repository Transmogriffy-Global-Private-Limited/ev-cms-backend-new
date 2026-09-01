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
- Source-controlled CPO permission catalog with role defaults and narrowly
  auditable per-membership ALLOW/DENY overrides; DENY takes precedence
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
- CPO transaction/session reads retain CMS session identity while exposing HAL
  and OCPP identifiers only as explicitly labeled protocol evidence.
- Runtime worker reporting distinguishes a logical worker role from its
  ephemeral process instance. The status API projects one authoritative current
  instance per logical worker; durable historical rows are not current health.
- Tariffs own commercial pricing only; active tax is resolved from the charger
  hub's same-CPO GST assignment. A zero price or tax rate is configured data,
  never evidence that a required tariff or GST is missing.
- A hubless charger is provisioning-only: it cannot be active or
  customer-visible, in application writes or PostgreSQL constraints.

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

- `docs/work/active/WI-20260812-cms-hal-operational-capabilities.md`.
- `docs/work/active/WI-20260826-customer-selected-session-limits.md`.

Approved current slice:

- Extend the existing customer start-intent, wallet-hold, HAL command, and
  charger-fact lifecycle with one optional customer-selected energy, time, or
  money limit. Tariff billing basis remains independent from the selected stop
  limit; no parallel charging flow is permitted.

Current implementation state:

- Mail-outbox template-catalogue correction is deployed under
  `docs/work/archive/WI-20260831-mail-outbox-template-catalog.md`. Migration
  058 is applied on the development database with historical rows preserved by
  its `NOT VALID` constraint.

- User App realtime projection correction is deployed under
  `docs/work/archive/WI-20260831-userapp-realtime-projections.md`: preserve the
  generic retained operational-event feed for compatibility while adding
  complete customer live-session and selected-charger SSE projections. The
  slice also corrects the CPO live-session watermark-before-snapshot ordering.
  Migration 058 and runtime deployment are recorded in the current project
  state; no direct data repair was required.

- CPO live-session snapshots and SSE payloads now expose
  `ocpp_transaction_id` from the durable transaction projection. Runtime
  revision `320d489` is deployed with migration 058; service/readiness,
  public routing, Swagger/OpenAPI (217 operations), Caddy, and post-rehost
   checks pass.

- Live charging financial and usage projections are deployed in runtime
  revision `d635446`. Customer and CPO reads use the shared immutable
  tariff/tax snapshot evaluator; projected amounts remain distinct from final
  settlement totals. No migration or data repair was required. Readiness,
  public routing, Swagger/OpenAPI (213 operations), Caddy, and post-rehost
  checks pass.

- The CPO customer aggregate wallet read is deployed in runtime revision
  `b6b723d`. Session usage/count and wallet balance are loaded independently,
  preserving stable zero values for missing rows; no migration or data repair
  was required. Readiness, public routing, Swagger/OpenAPI (213 operations),
  Caddy, and post-rehost checks pass.

- The CPO access, mail, subscription-notification, and support-product
  completion correction is implemented under
  `docs/plans/cpo-access-mail-support-completeness.md`. It replaces broad CPO
  ADMIN route gating with live capability decisions; establishes semantic mail
  templates and safe frontend links; delivers lifecycle notices through the
  durable outbox; and turns support into a bounded, auditable workflow.

- The support core, notification completion, and semantic mail correction are
  published on `main` and `anubhab-work` at
  `256aa8975fa07dc032dd779c8eb4b0d93a3b1a73`:
  migration 057, immutable lifecycle history, guarded transitions, locked
  idempotent replies, bounded cursor queue summaries, detail history, strict
  request decoding, OpenAPI, and PostgreSQL-gated integration coverage.
  Ticket creation, platform replies, and platform resolved/closed/reopened
  mutations now atomically queue privacy-safe mail intent; CPO activity is
  published through the durable platform-event stream and SMTP remains
  asynchronous. Migrations 56 and 57 are applied; runtime revision `b6b723d`
  is active with the 213-operation API. Readiness, public routing, Swagger,
  Caddy, and post-rehost startup checks pass.

- Customer-selected charging limits now retain independent threshold
  provenance through the existing start-intent/wallet-hold/HAL lifecycle.
  Migration 055 is applied; runtime revision `e8ff810` is active with the
  211-operation API. Readiness, public routing, Swagger/OpenAPI, Caddy, and
  post-rehost startup checks pass. Physical charger and disposable PostgreSQL
  coverage remain pending.

- The existing platform support desk now has a complete SuperAdmin workflow
  handoff. It makes the durable CPO-to-platform conversation boundary, full
  thread/list ordering, reply/status semantics, retries, privacy, and absent
  product features explicit; the OpenAPI contract now correctly declares that
  CPO support needs both its bearer and app-ID credentials. This is a
  documentation/contract correction only, with no runtime or data change.

- CPO live-session snapshots now include duration, display customer name, and
  canonical connector UUID in runtime revision `e6c0ebb`, with migrations
  49–53 still applied. No migration or data repair was required; public
  readiness, protected live-session access, and the 210-operation contract
  pass.

- CPO analytics period/date filters are rehosted in runtime revision `f432f45`
  with migrations 49–53 still applied. The OpenAPI contract documents the
  optional query parameters and 400 response; no migration or data repair was
  required. Public readiness, protected analytics access, and the 210-operation
  contract pass.

- The CPO live-session primary route is now full-snapshot SSE, with the JSON
  recovery snapshot at `/snapshot`, in runtime revision `d3ac043` with
  migrations 49–53 applied. The service is enabled behind Caddy, public
  readiness and docs routes pass, and the live OpenAPI contract contains 210
  operations. No new migration was required.

- Optional charger-provided SoC telemetry is deployed: HAL `transaction.soc`
  facts now project nullable first/latest SoC, observation time, and an
  independent sequence into charging sessions. Customer detail/history and
  operational invalidation expose observed SoC without estimating it. Migration
  48 is applied; disposable CMS/HAL/cpconsole acceptance remains pending.

- The current deployment also exposes enriched CPO charging-session list/detail
  projections, active-session live kWh overlays, and the SuperAdmin CPO
  customer-intelligence route. Session reads also hydrate an incomplete charger
  relation through the connector-owned relation. These changes require no
  additional migration; runtime revision `a085f29` is active with migrations
  45-54 applied. Migration 53 preserves three existing legacy CPO legal-
  identity rows with `NOT VALID` database checks while enforcing new and
  updated rows; the authoritative correction and later constraint validation
  remain a follow-up operation.

- Charging lifecycle repair is deployed: migration 45 gives connector occupancy to an
  unmaterialized start intent before materialization and to a single active,
  stop-pending, or reconciliation-required session afterwards. Completion
  settlement, wallet reservations, STOP reconciliation, and HAL transaction
  lookup now have explicit recovery invariants. Runtime revision `c6b79d4` is
  active with migrations 45-46 applied; disposable PostgreSQL occupancy tests
  remain skipped without `TEST_DATABASE_URL`.

- ChargingSession persistence correction is deployed: `TotalKWh` now has an
  explicit `column:total_kwh` mapping, backed by migration-aligned
  acronym-column and PostgreSQL-dialect dry-run insert regressions. The
  existing database schema is authoritative and unchanged; runtime revision
  `c6b79d4` is active with migrations 45-46 applied.
- HAL command-response contract hardening is deployed: malformed 2xx command
  bodies fail closed, HAL identity is never persisted as zero UUID, and
  authoritative start reconciliation repairs the historical zero sentinel as
  unknown. Runtime revision `c6b79d4` is active; migrations remain through
  `000046_auth_challenge_and_readiness_invariants.up.sql`. Disposable PostgreSQL recovery and
  dual-service physical acceptance remain required for full closure.
- Source now classifies deterministic HAL fact projection rejections as 4xx/409
  and uses the existing exact HAL transaction-by-start-intent socket to recover
  old unmaterialized starts through the same materializer as fact ingress. The
  remaining disposable-PostgreSQL and dual-service acceptance work is required
  before this is treated as deployed/verified.
- Charging-start reconciliation correction is deployed: mapping
  is a pre-command prerequisite, attempted-delivery ambiguity remains exact-ID
  reconciliation, and a typed exact HAL command 404 terminalizes only an
  unmaterialized START attempt and releases its HELD hold. STOP remains
  conservative. No migration was required; disposable PostgreSQL verification
  is still required before the work item can close.
- CMS source and the development deployment contain the first client, durable records, shared fact receiver,
  customer polling/start/stop routes, reusable operational projections,
  scoped operational-event replay/SSE, and the CPO live-session full-snapshot
  SSE/recovery routes. Runtime revision `e6c0ebb` is active with a 210-operation
  OpenAPI surface.
  The HAL runtime GORM models explicitly map to the singular migration tables
  `hal_charger_runtime` and `hal_connector_runtime`.
  The HAL v1 provider is
  not configured on this host, and the complete Postgres-to-HAL-to-virtual-
  charger vertical is not verified yet.
- The current deployed release also exposes tenant-scoped CPO charging-session
  list/detail reads with bounded keyset pagination and validated lifecycle
  filters. Revision `c6b79d4` is active with migrations 40-46 applied; the live
  OpenAPI contract contains 209 operations. The CPO wallet transaction read is
  tenant-scoped, newest-first, and cursor-paginated.
- The current deployed release also includes optional committed live charger
  projections on CPO charger list/detail responses. Revision `a5d1af4` is
  active; list reads use the bounded batch liveops reader.
- Migration 39 and the current-worker projection are deployed in revision
  `a5d1af4`; worker status/readiness now ignore superseded historical rows.
- The latest release adds tenant-scoped CPO customer usage/session/wallet
  aggregates and request-boundary tariff enum validation. No migration was
  required; the OpenAPI contract was reconciled to the registered runtime
  routes before rehost.
- The current deployed CMS release additionally contains the User App
  charging-history/detail completion slice: customer/CPO-scoped materialized
  session history, frozen commercial detail, linked settlement projection, and
  session-correlated operational invalidations. It is deployed in clean source
  revision `87b8727` without a database migration; the current start-admission
  hardening is deployed in revision `172bcd4` without a migration.
- The current deployed release includes state-aware GST-to-hub assignment and
  replacement validation from merge revision `4377383`, plus User App
  start-admission hardening in revision `172bcd4`: same-state hubs require
  SGST/CGST, different-state hubs require IGST, and new charging starts require
  fresh `AVAILABLE` connector state.
- The current deployed release includes additive migration thirty-six and the
  CPO charger customer-visibility gate. Customer discovery, direct lookup,
  pricing, and favorite paths require both the charger and published-hub
  gates; the live contract contains 178 operations.
- The current deployed release includes migration thirty-seven and the
  single-target tariff model. CPO tariff scope is derived from nested routes;
  User App hub/charger pricing and charging-start snapshots use explicit
  `USERGROUP > CHARGER > HUB` precedence. The active runtime is revision
  `a9fc32b`, and all tariff target constraints are present in the development
  database.
- The deployed CMS source hardens User App start admission without a
  migration, route, or HAL contract change: a new intent requires committed
  `AVAILABLE` and `FRESH` connector projection state, while same-customer
  active-intent replay remains available before that live-state gate. Connector
  row locking serializes the final active-intent recheck.
- Migration 40 introduced the canonical `price_per_unit` column without
  changing stored tariff values. The active tariff/GST correction now follows
  it with migration 42: energy units become `kwh` while retaining every numeric
  value as a per-kWh price, and Hub-owned GST integrity is enforced across
  assignment and later mutations. See
  `docs/plans/tariff-gst-commercial-correction.md`.
- Migration 40 and the tariff semantic correction are deployed in revision
  `9e7af67`: `price_per_unit` is interpreted explicitly as energy/watt-hour,
  time/minutes, or per-session across CPO writes, customer price, admission,
  snapshots, and settlement. The existing HAL/customer duration cutoff is
  unchanged.
- Migrations 41 and 43 and the customer aggregate correction are deployed in
  revision `ceefb21`; settings now retain wallet minimum/buffer defaults and
  every tenant customer receives wallet and zero-usage aggregate values.
- Migration 42 is deployed in the same revision: energy tariffs use the
  persisted `kwh` enum and Hub-owned GST assignment has a unique CPO/GST
  constraint.
- The current CPO release also exposes tenant-scoped analytics, hub-scoped
  charger listing, and charger-transaction reads; all are represented in the
  187-operation contract.
- Tariff PATCH intent and frozen GST settlement validation are deployed in
  revision `0ad2de7`; no migration was required.
- Temporal tariff fallback is deployed: migration forty-four replaces
  the former same-target no-overlap policy with root/open/bounded hierarchy,
  retains UserGroup > Charger > Hub as the primary selector, protects the
  customer-visible Hub root floor, and makes target identity immutable after
  creation. Visible Hub creation follows hidden Hub → root tariff → publish;
  resolver infrastructure errors remain 5xx rather than commercial absence.
  Its direct-DB Hub publication/root-topology race is serialized with the same
  `tariff:<cpo_id>:hub:<hub_id>` advisory transaction key. It is applied on
  the development database. The two requested hubs were removed with their
  hub tariffs and links; their five chargers and connectors remain unassigned,
  hidden, and inactive. The deployed service is revision `38625d9` with 186
  OpenAPI operations.
- CPO charger-transaction reads are deployed in revision `a5d1af4`; no
  migration was required.

Next required slice:

1. Add disposable PostgreSQL lifecycle tests for mapping, fact, projection,
   operational-event, and reconciliation persistence.
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
  operation requires its documented CPO capability and does not extend
  SuperAdmin authority.
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

Status: Implemented

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
system while retaining the retirement of platform billing. The observed
lifecycle worker expires elapsed periods, and that commercial fact narrowly
blocks only new customer charging starts and wallet recharge orders until a
SuperAdmin records renewal.

Non-goals:

- CPO administrative access control based on subscription status or dates
- Invoice, payment, checkout, collection, webhook, or provider integration
- Scheduled plan changes, period-end cancellation, automatic renewal, trial
  completion, or subscription email
- Feature keys, entitlement overrides, or feature enforcement before a module
  catalog and server-side gates are approved

Detailed plan:

- `docs/plans/manual-platform-subscriptions.md`

Architecture decision:

- `docs/decisions/0012-manual-platform-subscriptions-without-provider.md`
- `docs/decisions/0014-subscription-expiry-customer-command-admission.md`

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

Provide a correct ADMIN-led core CPO administration surface for administrator
identity profile, a read-only tenant organization projection, staff lifecycle,
hubs, chargers/connectors, GST, and tariffs without adding tenant-side
organization mutation or implying HAL integration.

Scope:

- Active CPO staff membership authentication; ADMIN enforcement for core CPO
  administration and provider-integration routes
- CPO staff lifecycle and source-controlled catalog/override data without
  frontend-only authorization
- CPO administrator identity profile read/update
- Session-bound, read-only CPO organization details
- Tenant-scoped network and pricing create/read/update operations
- Bounded charger listing and dependency-safe charger deletion
- Server-generated identifiers and exact decimal pricing/tax values
- Transactional audit evidence
- CPO legal-identity validation: checksum-valid GSTIN, GSTIN-state matching,
  six-digit Indian PIN code, and normalized human-readable CPO/admin fields
- OpenAPI, human contract, educational/integration guidance, and verification

Non-goals:

- App-user/customer changes
- Callable non-ADMIN core administration or provider-integration roles
- Tenant-side CPO organization mutation
- Complete CRUD for every table
- HAL handshake, live charger status, commands, callbacks, or tenant realtime

Detailed plan:

- `docs/plans/cpo-admin-network-configuration.md`

Agent handoff:

- `docs/CPO_BACKEND_AGENT_HANDOFF.md`
- `docs/CPO_FRONTEND_INTEGRATION_HANDOFF.md`
- `docs/SUPERADMIN_CPO_FRONTEND_BOUNDARY.md`
- `docs/contracts/api/superadmin-permission-matrix.md`

Architecture decision:

- `docs/decisions/0009-admin-only-cpo-authority.md`
- `docs/decisions/0015-cpo-legal-identity-validation.md`

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

- Phases 5 and 6: CMS HAL charging lifecycle and operational projections

Active feature:

- CMS HAL operational capability layer

Current implementation slice:

- Completed source-only CPO UAC authority coherence: protected CPO routes use
  their documented capabilities with active-membership/app-ID context, not a
  hard-coded ADMIN role. The slice distinguishes ordinary permission denial
  from evaluator infrastructure failure, aligns support and integrations,
  revokes only matching CPO sessions on an actual role change, preserves fresh
  SSE authorization, and verifies OpenAPI Bearer+App-ID AND semantics. Broad
  local Go verification and documentation/OpenAPI checks pass; PostgreSQL-gated
  lifecycle cases remain deferred without `TEST_DATABASE_URL`.
- CPO `chargers.operations` live-session operations: `GET /operations/live-sessions` is the
  full-snapshot SSE (initial `snapshot`, then replacement `live_sessions`
  frames) so the FE never reconstructs session state from invalidations. Each
  CPO-safe row carries `duration_seconds` at `as_of`, `customer_name`, and CMS
  `connector_id` alongside charger/hub/live telemetry. The explicit
  `/live-sessions/snapshot` JSON endpoint keeps recovery/keyset pagination;
  filtered event replay is advanced reconciliation only. The deployed contract
  has 211 operations; focused/broad source verification and live readiness pass.
- Completed source hardening: one-current authentication challenges are
  serialized by their identity owner and backed by partial unique indexes;
  administrative current-password changes lock before authorization; readiness
  is supplied by the current process's enabled expected-worker set rather than
  whichever durable rows happen to exist. Migration forty-six deliberately
  preserves charging migration forty-five and only repairs verified adjacent
  schema semantics. Full source checks pass; disposable PostgreSQL coverage
  remains pending an explicitly selected `TEST_DATABASE_URL`.
- Shared CMS HAL mapping/command reconciliation, fact ingress, durable
  projections, stale/offline freshness, scoped operational events, and CPO/App/
  Platform cursor/SSE consumption. Full User App charger list, hub detail,
  favorite, and single-detail projections now use one bounded live-state
  overlay, and the CPO HAL operational manual records the current capability
  and future command rules. PostgreSQL lifecycle and physical-topology
  acceptance remain pending.
- Current integration repair: preserve the committed HAL/liveops architecture
  while reconciling CPO changes, remove duplicate operational response types
  and create-path mapping delivery, align newly generated charger identities,
  and make migration 34/model tariff assignment metadata coherent. The
  serial-number HAL admission contract is now implemented in source as optional
  physical evidence; disposable PostgreSQL and hardware acceptance remain open.
- Current liveness repair: accepted HAL Heartbeats must renew durable ordered
  connection evidence; CMS reads connection freshness from a dedicated horizon,
  while meter freshness remains independent. REST recovery and realtime
  invalidation must converge on the same projected evidence.
- Tariff-domain correction is deployed in runtime revision `ebb57fb` after
  migration thirty-seven normalized legacy composite tariff rows to one target.
  CPO nested tariff CRUD fixes target scope, and User App hub price, charger
  price, and start-admission snapshots use one `USERGROUP > CHARGER > HUB`
  selector. The correction does not alter HAL/OCPP, wallet settlement, or
  session ownership.
- The commercial-tax follow-up is deployed through migration thirty-eight:
  tariff GST ownership is removed, hub GST is authoritative, and active or
  customer-visible chargers require a hub. The live contract remains at 178
  operations.
- The tariff/GST commercial correction is deployed: migration forty-two
  converts the energy enum to `kwh` without changing numeric tariff values;
  one shared pricing interpretation, Hub/GST mutation invariants, runtime GST
  defense, and the CPO frontend handoff are active. Disposable PostgreSQL
  lifecycle coverage remains pending an explicitly selected `TEST_DATABASE_URL`.
- Tariff PATCH and frozen-settlement hardening is deployed in revision
  `0ad2de7`: omitted,
  null, and value semantics are explicit for mutable units/schedule fields;
  all three tariff scopes apply and validate the resulting row through one
  shared helper; immutable GST snapshots validate their own commercial
  components without a current Hub/GST lookup. No migration is required.
- Wallet admission policy is deployed: migration forty-three backfills a blank
  settings row for every existing CPO while new CPO provisioning creates the
  same zero-default row. Each new start locks and enforces the CPO minimum and
  buffer before deriving its hold and HAL energy limit; the independent
  duration-cutoff workflow is unchanged. Customer wallet/history reads expose
  the current policy, usable balance, and threshold recharge shortfall.

Last completed slice:

- Deployed tariff PATCH nullability and frozen GST settlement hardening in
  revision `0ad2de7`; no migration was required.

Previous completed slice:

- Wallet admission policy migration forty-three and its tenant-scoped wallet
  projections are deployed and documented above.
- Tariff/GST commercial correction: corrected energy per-kWh semantics across
  tariff writes, customer price, admission, immutable snapshots, and
  settlement; protected the complete Hub/GST relationship across later
  mutations; and published the CPO frontend integration handoff.
- Hardened tariff PATCH intent and frozen settlement: `units:null` clears the
  session basis units, paired null dates clear a schedule, and malformed GST
  components in a historical snapshot fail safely without mutable GST reads.
- Completed User App consumption of committed HAL operational projections for
  every full charger response without N+1 reads, preserving compact map-only
  location payloads, and added the canonical CPO backend HAL operational
  capability manual. Full dual-service acceptance remains pending.
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
- Added a compact User App charger-location list that reuses those published
  charger filters and pagination rules while returning only `charger_name` and
  attached-hub latitude/longitude for map markers.
- Implemented the effective customer tariff resolver and informational hub and
  charger price APIs with exact decimal projections, explicit unavailable state,
  active GST resolution, and User Tariff > charger tariff > hub tariff
  precedence.
- Corrected the tariff model from mandatory hub context plus optional composite
  fields to exactly one durable target. Migration thirty-seven backfills legacy
  rows with `usergroup > charger > hub` precedence, adds matching
  `assigned_to`/target constraints, and makes the target foreign keys CPO-safe.
  The CPO nested routes now supply that fixed target; the User App selector and
  charging snapshot reuse the same fixed precedence. This deployed slice has
  passed Go tests/vet and live migration/constraint verification; disposable
  PostgreSQL lifecycle verification remains guarded by `TEST_DATABASE_URL`.
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
  effective dates and migration-fifteen exclusion enforcement; migration
  thirty-seven later supersedes its former mandatory-hub target representation
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

- Runtime source revision `ebb57fb` was built and rehosted after migration
  thirty-eight. The installed binary has SHA-256
  `0ea00edc15477c5d7c90c66304bbf001799a952a00fdd01f3e0723e7f6e4aa3b`, the
  live contract has 178 operations, and the service is healthy with zero
  restarts. Migration/constraint checks, Caddy validation, loopback/public
  health and readiness, Swagger, raw OpenAPI, protected tariff/customer-price
  boundaries, and the post-rehost journal scan passed. Disposable PostgreSQL
  lifecycle and full HAL/virtual-charger acceptance remain pending.
- Runtime source revision `a9528c4` was built and rehosted after migration
  thirty-six added the charger customer-visibility publication gate. The
  installed binary has SHA-256
  `09b80b12865866b84e8a690bbaa2829257a036af8b2fee5c69fb3a7808a4b60c`, the
  live contract has 178 operations, and the service is healthy with zero
  restarts. Migration/column checks, Caddy validation, loopback/public health
  and readiness, Swagger, raw OpenAPI, the unauthenticated charger-visibility
  boundary, and the post-rehost journal scan passed. Disposable PostgreSQL
  lifecycle and full HAL/virtual-charger acceptance remain pending.
- Clean source revision `172bcd4` was built and rehosted without a migration for
  User App charging-start admission hardening. The installed binary has SHA-256
  `ab53143ae0bb55d14e9256d77eb5bf3350ce1aed2c280236b41dc4ad80ea2238`, the
  live contract remains at 177 operations, and the service is healthy with zero
  restarts. Migration/table checks, Caddy validation, loopback/public health and
  readiness, Swagger, raw OpenAPI, the unauthenticated charging-start boundary,
  and the post-rehost journal scan passed. Disposable PostgreSQL lifecycle and
  full HAL/virtual-charger acceptance remain pending.
- Clean merge revision `4377383` was built and rehosted without a migration for
  state-aware GST-to-hub validation. The installed binary has SHA-256
  `769b71782f47bb93c37e063dbab4ba8af34902b80ac8fb3150c21ac61c2fc5e0`, the
  live contract remains at 177 operations, and the service is healthy with zero
  restarts. Migration/table checks, Caddy validation, loopback/public health
  and readiness, Swagger, raw OpenAPI, GST route boundaries, and the post-
  rehost journal scan passed. Disposable PostgreSQL lifecycle and full
  HAL/virtual-charger acceptance remain pending.
- Clean source revision `87b8727` was built and rehosted without a migration
  for the User App charging-history release. The installed binary has SHA-256
  `007d40cbd9eeda79392f7b1d546cc4d6e2bf336913212c6ce9f17dde4f9a6434`, the live
  contract has 177 operations, and the service is healthy with zero restarts.
  Migration/table checks, Caddy validation, loopback/public health and
  readiness, Swagger, raw OpenAPI, the unauthenticated history-route boundary,
  and the post-rehost journal scan passed. Disposable PostgreSQL lifecycle and
  full HAL/virtual-charger acceptance remain pending.
- Clean source revision `0d50c09` was built and rehosted without a migration
  after the HAL runtime GORM table-name correction. The installed binary has
  SHA-256 `e3790854e68f7a3996d50a552e2f15ef6a95f644184e10389a95d513d64b24bf`,
  the live contract remains at 176 operations, and the service is healthy with
  zero restarts. Migration/table checks, Caddy validation, loopback/public
  health and readiness, Swagger, raw OpenAPI, and the post-rehost journal scan
  passed. The disposable PostgreSQL lifecycle and full HAL/virtual-charger
  acceptance remain pending.
- Revision `3f3a952` previously introduced migration thirty-three and durable
  operational events; revisions `2e8fdb3` and `e831b32` subsequently applied
  migrations thirty-four and thirty-five.
- Revision `27c01f3` subsequently rehosted the shared bounded User App
  live-state overlay and its CPO HAL operational manual without a migration.
  Revision `2e8fdb3` then applied migration thirty-four for nullable tariff
  assignment metadata and rehosted the reconciled CPO/HAL integration.
  Revision `e831b32` subsequently applied migration thirty-five for same-CPO
  GST-to-hub assignment and rehosted the four-route CPO contract. The service
  remains healthy with zero restarts; loopback/public health, Swagger, raw
  OpenAPI, and the 176-operation contract were verified.

Next expected slice:

- Design the CMS/HAL QR-scan charging lifecycle before adding another customer
  start/stop entry point.

Blocked by:

- None

## Next Approved Work

1. Design the CMS/HAL QR-scan charging lifecycle before adding another
   customer start/stop entry point.

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
- CPO roles are source-controlled default permission bundles. Endpoint authority
  is the route's documented capability, evaluated against the current active
  membership and overrides; explicit `DENY` wins. Custom roles and hub-scoped
  delegation remain deferred.

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
