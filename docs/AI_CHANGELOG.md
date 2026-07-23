# AI Changelog

## 2026-07-23

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
