# AI Changelog

## 2026-07-24

### Tenant commercial management retired

- Replaced the tenant subscription, entitlement, platform-invoice, and
  platform-payment direction with manual superadmin CPO activation/suspension.
- Removed the subscription and platform-billing runtime modules, routes,
  models, workers, mail producers, OpenAPI operations, and active contracts.
- Added migration nine, which preserves the already-deployed prototype tables
  in `retired_commercial` rather than dropping data.
- Migration nine disables retired worker requirements and refuses to proceed
  while a related mail job is pending or processing.
- Added ADR 0008 and rewrote the superadmin plan, concepts, API handoff, schema
  ownership, and project state around the simpler access model.

Verification:

- Migration nine passed disposable PostgreSQL 17 up, pending-mail guard,
  preserved-row archival, worker disablement, down restoration, and worker
  re-enable checks.
- Retired routes return `404`; all 44 runtime/OpenAPI operations match.
- Focused tests, full `go test ./...`, `go vet ./...`, documentation
  verification, residue scans, and `git diff --check` passed.

### Development VPS updated to the control-plane release

- Fast-forwarded the clean `main` checkout from `d610efe` to `e15699d`.
- Added the new non-secret API documentation and platform worker settings to
  the ignored deployment environment while preserving its existing secrets.
- Built and checked a replacement binary before changing the running service.
- Created a mode-0600 pre-migration PostgreSQL custom-format backup at
  `/tmp/devevcmsnewdb-pre-e15699d.dump`.
- Applied forward migrations six through eight, bringing the migration ledger
  to eight versions and the development schema to 42 public tables.
- Preserved the previous binary as `builds/evcmsnew.d610efe`, installed the new
  binary, and invoked the shared `rehost-service` handler.

Verification:

- Deployment configuration loading, focused runtime/OpenAPI verification,
  `go test ./...`, `go vet ./...`, the Bash-equivalent documentation contract
  check, and `git diff --check` passed.
- The service is enabled and active with zero restarts, and systemd is running
  the exact newly built binary.
- Loopback and public liveness/readiness, Swagger UI, and raw OpenAPI passed.
- The new platform worker API rejects unauthenticated access with `401`.
- Platform maintenance, subscription lifecycle, billing maintenance, and mail
  outbox workers registered healthy.
- All representative platform operations, subscription, and billing tables
  exist, and the post-rehost journal contains no error-level entries.
- Granular PostgreSQL lifecycle integration tests remain pending because this
  deployment database is not an authorized disposable test target.

### Complete superadmin control plane approved and started

- Inventoried the implemented new-CMS platform, auth, CPO, mail, audit, and
  documentation surfaces.
- Inspected the legacy CMS data model and route behavior instead of treating
  its mixed admin route names as authoritative.
- Confirmed the legacy CMS has no durable subscription model to preserve.
- Approved a provider-neutral subscription and entitlement model in which CPO
  creation and lifecycle remain independent from subscription state.
- Approved complete platform operations for CPO/admin recovery, platform
  administrators, subscription billing records, audit/security, mail,
  notifications, workers, announcements, overview, system status, and durable
  SSE realtime with REST replay.
- Recorded the binding implementation plan and ADR 0007.
- Added the required `API_DOCS_ENABLED` configuration toggle for Swagger UI and
  raw OpenAPI route registration while preserving the enabled compatibility
  default.

Implementation:

- Focused configuration and route tests passed for documentation enabled and
  disabled behavior.
- Implemented durable platform events, audit queries, worker heartbeats,
  readiness degradation, REST replay, and authenticated SSE.
- Added versioned subscription plans, database-immutable published snapshots,
  exact pricing, entitlements, optional CPO assignments, complete lifecycle
  transitions, scheduled boundary changes, idempotent history, overrides,
  CPO-admin mail, audit, events, and lifecycle reconciliation worker.
- Added provider-neutral platform billing accounts, immutable issued invoice
  terms/lines, exact arithmetic, payment allocation and reversal, timeline
  queries, overdue reconciliation, invoice mail, audit, and durable events.
- Expanded OpenAPI to all 74 runtime operations and added granular realtime,
  subscription, and platform-billing contracts.

Verification:

- Focused Go tests, OpenAPI semantic validation, route/OpenAPI parity, route
  protection, model parsing, migration discovery, mail tests, and documentation
  verification passed.
- Full `go test ./...`, `go vet ./...`, documentation verification, and
  `git diff --check` passed after the billing slice.
- Disposable PostgreSQL execution for migrations six through eight and their
  lifecycle tests remains pending.

## 2026-07-23

### Development VPS hosting prepared and activated

- Prepared `dev-evcmsnew.transev.site` behind Caddy with an application
  listener reserved on `127.0.0.1:18080`.
- Created the additive PostgreSQL database `devevcmsnewdb`, owned by
  `postgres`.
- Created an ignored mode-0600 `.env` from `.env.example`, set the requested
  initial superadmin identity, and generated five independent cryptographic
  keys.
- Added `evcmsnew-dev.service`, centralized ignored `builds/` output, and the
  `rehost-evcmsnew` interactive handler.
- Added the supplied database and SMTP passwords only to the ignored mode-0600
  deployment environment.
- Applied all five forward migrations, bootstrapped the configured platform
  superadmin, and enabled and started the application service.
- Added the authoritative development-hosting and activation guide.

Verification:

- Caddy configuration validation and reload completed.
- The database owner, loopback listener, DNS resolution, binary build,
  documentation checks, Go tests, and Go vet were verified.
- PowerShell is not installed on this VPS, so `scripts/verify-docs.ps1` could
  not run directly; its required-file, route-presence, and removed-setting
  checks were reproduced in Bash and passed.
- The migration ledger contains all five versions and 29 public tables.
- The service is enabled and active; loopback and public HTTPS liveness and
  database-readiness checks passed.
- A platform-login request returned `202`, confirmed the bootstrapped
  superadmin, and produced a real `LOGIN_OTP` delivery that reached `SENT` on
  the first attempt.

### Temporary remote-development access implemented

- Added environment-controlled permissive CORS middleware with preflight
  handling for remote browser frontends.
- Set the current ignored `.env` and checked-in `.env.example` to listen on
  `0.0.0.0:8080` with `CORS_ALLOW_ALL=true`.
- Kept the code defaults at loopback-only and CORS-disabled when those
  variables are absent.
- Preserved authentication, CPO app-ID validation, tenant isolation, and
  authorization for all actual API requests.
- Documented the remote client URL, operational risk, and production rollback.

Verification:

- Configuration and route tests cover enabled and disabled CORS behavior.
- Documentation verification, runtime/OpenAPI route matching, `go test ./...`,
  and `go vet ./...` passed.

### Complete app-user authentication boundary implemented and verified

- Approved a distinct `CUSTOMER` session scope tied to one CPO customer.
- Defined app password-plus-mail-OTP login, encrypted access tokens, rotating
  refresh tokens, `me`, scoped session management, logout, and password
  operations.
- Defined trusted backend helpers for current user, customer, CPO, and app
  identity.
- Kept customer session operations isolated from administrative and other-CPO
  sessions while retaining global password revocation semantics.
- Added the `000005_customer_authentication` migration with a database-enforced
  user/customer/CPO session identity and customer challenge/mail contracts.
- Added customer login verify/resend, refresh, `me`, session
  list/revoke/logout, password recovery/reset/resend/change, and protected app
  middleware.
- Added trusted `customerauth.CurrentPrincipal`, `CurrentUserID`,
  `CurrentCustomerID`, `CurrentCPOID`, and `CurrentCPOAppID` helpers.
- Expanded the human API contract and canonical OpenAPI to all 40 operations.

Verification:

- Migration down, up, and idempotent-up passed in PostgreSQL 17.
- Customer authentication lifecycle passed for login OTP, encrypted access,
  `me`, refresh rotation/reuse, session isolation/revocation, recovery/change,
  and global password session revocation.
- Documentation verification and all 40 runtime/OpenAPI operation matches
  passed.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.

### CPO-scoped customer signup implemented and verified

- Approved public email-verified signup under an active CPO app identity.
- Kept `X-CPO-App-ID` as public routing metadata rather than authentication.
- Defined durable pending challenges that retain only a password hash and
  HMAC-protected OTP.
- Defined safe global identity reuse without password or profile overwrite.
- Defined transactional customer and zero-balance INR wallet creation without
  a subscription dependency.
- Kept customer login and session issuance outside this slice.
- Added the `000004_customer_signup` up/down migration, CPO-bound challenge
  model, public start/verify/resend handlers, mail template, audit write, and
  transactional customer/wallet creation.
- Added explicit human and OpenAPI contracts plus runtime/spec drift coverage.

Verification:

- PostgreSQL migration down, up, and idempotent-up passed.
- PostgreSQL signup lifecycle passed for resend invalidation, replay rejection,
  global identity creation/reuse, password/profile preservation, CPO isolation,
  and zero-balance INR wallet creation.
- Documentation verification and all 27 runtime/OpenAPI operation matches
  passed.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.

### Hostinger SMTP contract and durable documentation surfaces

- Replaced the string SMTP mode with mutually exclusive
  `SMTP_USE_SSL`/`SMTP_USE_TLS` flags.
- Configured the checked-in example for `team@transev.in` through
  `smtp.hostinger.com:465` using implicit TLS, while keeping the password
  environment-only.
- Rejected plaintext, conflicting transport modes, and half-configured SMTP
  credentials during startup validation.
- Added focused Hostinger configuration and SMTP-construction tests.
- Added a canonical documentation map, educational identity/onboarding/support
  guides, external integration records, and API/internal/configuration
  contracts.
- Registered the new documentation surfaces in repository instructions and
  added a PowerShell documentation verifier.
- Expanded the human API contract to document every implemented endpoint,
  request/response, authorization rule, validation, state effect, and error.
- Added canonical OpenAPI 3.1, embedded same-origin Swagger UI, raw spec
  serving, semantic validation, and bidirectional runtime-route drift tests.
- Expanded educational, SMTP, Razorpay, HAL, mail-outbox, and configuration
  documentation to integration-handoff detail.

Verification:

- Focused configuration and mail tests passed.
- Documentation contract and removed-configuration residue checks passed.
- OpenAPI parsed and passed semantic validation.
- All 24 implemented business/health operations matched the runtime router and
  OpenAPI in both directions.
- Embedded Swagger UI, redirect, and raw-spec endpoints passed HTTP smoke tests.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- Real Hostinger delivery was not attempted and remains an operational check.

### CPO provisioning and app identity verified

- Approved CPO creation without a subscription dependency.
- Defined a server-generated dummy app ID at creation and a separate
  superadmin-controlled live app-ID transition.
- Kept lifecycle status independent from dummy/live app-ID status so onboarding
  can run with an active CPO and dummy ID.
- Defined `X-CPO-App-ID` as tenant-routing metadata that must match the
  authenticated CPO but never replaces user/session authorization.
- Exempted health, authentication, platform control-plane, and future
  independently authenticated callback surfaces from the tenant app-ID header.
- Expanded creation to atomically establish the first CPO admin.
- Defined encrypted delivery of a generated temporary password for a new
  identity and safe existing-identity reuse without password overwrite.
- Defined durable first-login password-change enforcement with no expiry and a
  reminder after each successful login until the password changes.

Verification:

- Architecture and implementation plan recorded.
- Added the `000003_cpo_provisioning` up/down migration and embedded migration
  coverage.
- Added platform CPO create/list/get/activate/suspend/live-app-ID APIs.
- Added atomic first-admin membership, encrypted onboarding or assignment
  email, non-expiring temporary-password change enforcement, and login
  reminders.
- Serialized concurrent first-admin identity creation per normalized email.
- Added current app identity to CPO login, refresh, and `me`, plus
  `X-CPO-App-ID` enforcement on tenant business APIs.
- Added session and refresh-token revocation on CPO suspension.
- Focused and PostgreSQL lifecycle tests passed.
- Migration down, up, and idempotent up passed on PostgreSQL 17.
- Full PostgreSQL-backed `go test ./...`, `go vet ./...`, residue scanning, and
  `git diff --check` passed.

### Authentication and credential boundary started

- Approved one shared credential boundary for platform superadmins and CPO
  staff while preserving separate authorization scopes.
- Defined an idempotent environment-only first-superadmin bootstrap that never
  overwrites an existing administrator password.
- Selected mandatory email OTP for administrative login, a durable PostgreSQL
  mail outbox worker, encrypted access tokens, rotating opaque refresh tokens,
  session revocation, password recovery, and trusted tenant helpers.
- Defined CPO-admin write-only encrypted Razorpay credentials without
  superadmin plaintext access.
- Kept public signup, social login, SMS/TOTP/passkeys, custom RBAC, Redis,
  external queues, and payment execution outside this slice.

### Authentication and credential boundary verified

- Added the additive `000002_auth_credentials` up/down migration for user
  security state, challenges, sessions, refresh lineage, encrypted mail jobs,
  durable rate limits, and tenant integration credentials.
- Added Argon2id password handling, independent OTP/secret encryption, and
  fixed-algorithm signed-then-encrypted access JWTs.
- Added concurrency-safe idempotent environment bootstrap without existing
  password overwrite.
- Added platform/CPO password-plus-email-OTP login, resend, refresh rotation,
  refresh-reuse revocation, identity/session APIs, password recovery/change,
  and authorization helpers.
- Added a retrying PostgreSQL outbox worker with mandatory SMTP TLS.
- Added CPO owner/admin Razorpay credential create/rotate, metadata, delete, and
  internal resolve behavior. APIs never return secret plaintext.
- Added complete API/configuration documentation and focused plus PostgreSQL
  integration tests.

Verification:

- Focused cryptography, auth, worker, integration-secret, configuration, model,
  and route tests passed.
- PostgreSQL 17 migration down, up, and idempotent up passed.
- PostgreSQL authentication, recovery, mail-worker, and credential-isolation
  integration tests passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- Real SMTP-provider delivery was not run because no provider credentials were
  supplied.

### Lean CMS foundation started

- Replaced the empty scaffold concept with a documented multi-tenant CPO CMS
  boundary.
- Kept the OCPP HAL explicitly separate from the CMS and deferred the handshake
  contract to the relevant implementation phase.
- Chose fixed CPO-wide roles instead of custom RBAC or per-hub scopes.
- Defined the initial platform-admin, user, CPO membership, and customer data
  model.
- Added versioned migration, database startup, and health-service foundations.
- Recorded custom roles, plan entitlements, RLS, event infrastructure, charging,
  and finance as deferred work rather than speculative implementation.

Verification:

- Go formatting completed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- PostgreSQL migration integration remains unverified because no explicitly
  selected disposable development database was used.

### Complete supplied CMS schema restored

- Corrected the initial over-simplification that omitted most supplied CMS
  domain data.
- Added user settings, tenant user groups, hubs, chargers, connectors, group
  access links, customer favorites, GST profiles, tariffs, wallets, wallet
  transactions, charging sessions, payments, and audit logs.
- Preserved CPO profile data on the CPO tenant and mapped app users to
  tenant-scoped customer records.
- Added CPO identifiers and tenant-matching foreign keys throughout the owned
  domain.
- Added exact-decimal Go types and PostgreSQL numeric fields for finance.
- Added matching `000001_cms_schema.up.sql` and
  `000001_cms_schema.down.sql` migrations.
- Added an explicit latest-migration rollback operation and migration command.
- Added tests for complete table coverage, up/down pairing, JSONB behavior, and
  GORM model parsing.

Verification:

- Focused migration, model, JSONB, and constants tests passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- PostgreSQL 17 up migration created all 21 domain tables.
- A second up run was idempotent and retained one migration version.
- Down migration removed all domain tables and retained only
  `schema_migrations`.
- The loopback-only disposable PostgreSQL instance was stopped and its local
  test directory was removed.
