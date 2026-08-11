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
- tenant-scoped, credential-owning customer accounts;
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
  the reset handler while the forgot response remains generic, and every OTP
  producer uses the canonical complete mail payload without a lossy wrapper;
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
- a canonical SuperAdmin frontend handoff covering the 66-operation current
  platform integration surface, TypeScript contracts, auth/token state, CPO
  workflows, governance, security, mail, notifications, overview/status,
  audit/workers, SSE/replay, error UX, security, verification, and explicit
  deployment gaps;
- canonical OpenAPI 3.1 for all 135 current source-tree business/health
  operations;
- embedded same-origin Swagger UI at `/docs/` and raw OpenAPI at
  `/openapi.yaml`;
- CPO Swagger operations organized into Account & Notifications, Network,
  Pricing & Tax, and Integrations sections without mixing platform or User App
  operations;
- `API_DOCS_ENABLED` registration control for both documentation surfaces,
  defaulting on for compatibility and returning `404` when disabled;
- bidirectional verification that Gin and OpenAPI expose the same operation
  set;
- public CPO-scoped customer signup start, verify, and resend APIs;
- durable signup challenges with hashed pending passwords and HMAC-protected
  OTPs;
- transactional CPO-local customer-account and zero-balance INR wallet
  creation without a global administrative user;
- dedicated customer challenge, session, and refresh-token tables bound to one
  customer and CPO without a staff role or global-user foreign key;
- customer password-plus-mail-OTP login, signed/encrypted access tokens, and
  rotating/reuse-detecting refresh tokens;
- app-user `me`, customer-scoped session listing/revocation/logout, CPO-local
  password reset/change, and eligible-recipient recovery-ID/code delivery;
- authenticated customer self-service profile updates through
  `PATCH /api/v1/app/profile`, with omitted-versus-null phone semantics,
  canonical user projection responses, and CPO-scoped field-name-only audit
  evidence;
- CPO ADMIN-controlled default-false hub publication through
  `customer_visible`, plus authenticated customer-safe published network
  discovery for hubs, attached chargers, and connectors; the hub
  `open_24_hours` and charger `twenty_four_seven_open_status` values are
  separate, connector total capacity and static CMS administrative statuses are
  returned separately from HAL-owned live availability, which remains
  `UNKNOWN`;
- customer-owned favorite list and idempotent add/remove APIs over published
  hubs and attached chargers, with unpublish-safe reads and CPO/customer
  composite ownership;
- authenticated informational hub and charger price resolution using active
  effective tariffs, active GST projections, explicit `AVAILABLE`/
  `UNAVAILABLE` states, and User Tariff > charger tariff > hub tariff
  precedence;
- authenticated User App charger search/filter and bounded near-me reads over
  published hubs, with safe hub, display/category/parking charger, and
  connector projections; DB-backed status and connector total capacity; an
  authenticated charger-image route keyed by public charger ID; and explicit
  UNKNOWN live availability;
- authenticated CPO/customer-scoped wallet balance and keyset-paginated wallet
  history reads using exact decimal projections;
- User App Razorpay recharge order creation and captured-payment verification
  through the existing encrypted CPO integration credentials, with migration
  twenty-two durable recharge orders, provider payment attempts, future-refund
  records, provider snapshots, signature evidence, and atomic wallet-credit
  ledger linkage; no CPO/Superadmin payment APIs, refund execution, webhook,
  settlement reconciliation, RFID, or HAL integration;
- trusted backend current-principal, customer, CPO, and app-ID helpers, with
  `CurrentUserID` retained as a customer-ID compatibility alias;
- a separated User App route topology: credential/session operations remain
  under `/api/v1/app/auth`, while authenticated app resources (`me`, profile,
  discovery, favorites, pricing, wallet, and recharge) are under
  `/api/v1/app`;
- environment-controlled permissive CORS middleware and a current development
  configuration that listens on all IPv4 interfaces for access from other
  machines;
- durable platform event replay, authenticated SSE, filtered audit query, and
  registered worker-health/readiness APIs;
- complete source-tree platform-superadmin governance: administrator
  invite/grant/activate/deactivate with last-active-admin protection;
- source-tree security operations for locked identities, reasoned unlock,
  security-event visibility, and scoped user session revocation;
- source-tree safe mail-job administration for metadata listing, retry/cancel,
  metrics, stale-job reconciliation, and audited bounded retention;
- source-tree platform/CPO announcements with immutable audience snapshots and
  durable recipient notifications, including recipient-owned read state;
- source-tree bounded platform overview and service-status aggregates;
- platform-superadmin-only manual subscription plans, immutable published plan
  versions, explicit CPO issue/renew/change/status commands, and idempotent
  transition history;
- server-generated subscription UUIDs/version numbers/timestamps plus atomic
  audit/platform-event records; no client supplies those values;
- no provider, checkout, invoice/payment API, webhook, subscription mail,
  automatic renewal, scheduled transition, or lifecycle worker; CPO
  activation/suspension remains independent from subscription state;
- a reversible migration-nine retirement boundary that preserves the removed
  prototype tables in `retired_commercial` and disables their worker records;
- an active VPS deployment at `dev-evcmsnew.transev.site`, with Caddy proxying
  to the loopback-only listener `127.0.0.1:18080`;
- an enabled and active `evcmsnew-dev.service`, ignored mode-0600 deployment
  environment, compiled binary layout, and `rehost-evcmsnew` interactive
  handler;
- the additive PostgreSQL database `devevcmsnewdb`, owned by `postgres`.

The active development VPS runs source revision `a76d6ae`, with migrations
through thirty-one recorded and the deployed 160-operation contract. Migration
twenty-nine adds nullable `tariff_type`, `price_type`, and `units` metadata to
tenant tariffs; omitted values remain null-safe for existing and newly created
tariffs. The SuperAdmin administrator-list query explicitly binds the platform
administrator model. The User App can serve an authenticated published charger's allowed image through
its relative `charger_image_url`, without exposing the stored upload path.
The CPO charger response also exposes a read-only `assigned` projection that
matches hub attachment, and Swagger groups CPO operations by account/network,
pricing/tax, and integration responsibilities.
The CPO ADMIN customer directory is read-only and tenant-scoped, and CPO
charger projections expose `hub_name` when assigned. Connector capacity is
stored and exposed as `connector_total_capacity` in CPO create/update requests
and response projections, as well as User App connector projections.
User App hub summaries expose `open_24_hours`; charger projections separately
expose the charger's `twenty_four_seven_open_status` and the attached hub's
`hub_open_24_hours`.
CPO hub, charger, and user-group tariff routes are tenant-scoped; CPO user-group
CRUD is protected by the same administrative authorization boundary.
The user-group member-assignment and member-removal operations are
tenant-scoped and idempotent; membership changes record an audit action, and
the user-group detail response exposes safe `members` customer projections;
customer projections expose `usergroup_assigned`. The tenant-scoped CPO settings
API exposes invoice-note and invoice-logo metadata through authenticated GET,
POST, and PUT routes; migration thirty stores one settings row per CPO. A
GST profile now has a required API-level `state` value; migration thirty-one
adds the nullable durable column and permits legacy GST-rate values to remain
null. New GST creation continues to require all three validated rates. A
connected platform realtime SSE
client can make graceful shutdown reach its bounded timeout during rehost;
systemd restarts the service automatically and health checks then pass.
Migration
twenty-seven replaces legacy charger/connector protocol-style states with the
static CMS administrative values `ACTIVE`, `INACTIVE`, `SUSPENDED`,
`UNDERMAINTENANCE`, and `DECOMMISSIONED`. CPO GSTIN and address
identity are database-required, authenticated platform clients can use the
advisory slug-availability operation, and known uniqueness races return
field- or relationship-specific conflict codes. The current database contains
two CPO records. Migration nine continues to preserve the
`retired_commercial` schema. Safe structured HTTP request logging is active;
the current development environment uses `LOG_LEVEL=DEBUG` for correlated
request-start and completion diagnostics.

Migration twenty-eight adds the CMS-owned charging start-intent, wallet-hold,
HAL-command, fact-receipt, charger-mapping, and charger/connector runtime
projection state. The current deployment exposes the first customer charging
start/stop/status and HAL fact-receiver routes, but its optional HAL v1 base URL
and credentials are unset, so customer charging remains unavailable until the
approved independent provider is configured.

Migration thirteen removes feature-key runtime behavior and is deployed.
Migration fourteen completes the deployed Superadmin control-plane surface.
Migration fifteen adds tariff effective-date fields and a tenant/scope-aware
PostgreSQL exclusion constraint and is deployed. The disposable PostgreSQL
lifecycle test remains unexecuted because no `TEST_DATABASE_URL` is configured.

The deployed contract has 129 operations: the added
`GET /api/v1/cpo/users/{user_id}` is a tenant-scoped staff-membership point
lookup, not a customer or staff directory. CPO-local customer accounts are not
reachable through it.

The deployed source has 129 operations. It includes
operations after adding customer self-service profile editing, published
network discovery, favorites, informational customer price reads, charger
search and wallet reads, and Razorpay recharge order/verification; the deployed binary remains at 113 until a
separately approved deployment. The deployed
source includes the CPO ADMIN-only
`POST /api/v1/cpo/hubs/{hub_id}/chargers` hub attachment/reassignment command,
allows an independent charger to be created without `hub_id`, and adds
non-negative hub `sanction_load` plus the upgrade-time removal of the legacy
charger-hub `NOT NULL` in migration sixteen. These changes are deployed at
`782dd7b`; migration nineteen reconciles databases that had already recorded
the removed follow-up migrations so `chargers.hub_id` is nullable, and migration
twenty makes customer accounts CPO-local with dedicated authentication lineage,
and migrations 21–22 add customer-visible network discovery and Razorpay wallet
recharge ledger support. Migration twenty-six removes obsolete connector
current/voltage fields; connector capacity is represented by
`connector_total_capacity` in CPO create/update requests and response
projections, and in User App connector projections.

The deployed recovery flow fixes a recovery-specific OTP mapper defect that discarded
`challenge_id` before outbox validation and caused eligible administrative and
customer forgot-password transactions to roll back with `500 internal_error`.
Administrative and customer recovery now enqueue the complete canonical mail
payload before outbox validation.

CPO customer-app implementation requires `X-CPO-App-ID` on every
`/api/v1/app/...` request, including signup. The approved next user-work
plan retains that app-only header. Customer self-service name and phone
editing, published-station discovery, favorites, and informational tariff
display are implemented in source; HAL-dependent charging/billing work remains
planned.
CPO ADMIN routes remain owned by the CPO workstream.

The CMS/HAL transport, authenticated fact receiver, durable charging intent and
hold state, and customer charging start/stop/status routes are implemented and
deployed. Full HAL handshake, live charger state ingestion, virtual-charger
acceptance, restart/outage recovery, reconciliation worker, Razorpay
refund/webhook/settlement workflow, tenant commercial-management workflow,
staff-management workflow, and reporting behavior remain incomplete or
intentionally unsupported.

## Verification

- Go formatting completed.
- Safe request completion logging, secret/content exclusion, loopback-only
  forwarded-address trust, handled error correlation, authentication failure,
  safe recovered-panic diagnostics, stock request-dump suppression, and CORS
  request-ID exposure have focused test coverage.
- DEBUG request-start and handled-error diagnostics have focused mode,
  correlation, classification, and secret/content leak coverage.
- Complete direct OTP payloads and both administrative/customer recovery
  template validations have database-free regression coverage. The changed
  PostgreSQL recovery lifecycle was not run because no disposable
  `TEST_DATABASE_URL` was configured.
- Known CPO unique-constraint mappings and the unknown-constraint fallback have
  focused unit coverage; PostgreSQL lifecycle assertions now require the exact
  slug and GSTIN conflict codes.
- Required-field validation, slug normalization/authorization, migration
  content, and affected package tests passed for the source-tree change.
- Superadmin migration fourteen static coverage, input/privacy regression
  tests, and the affected package tests passed for the source-tree change.
- The 124-operation source OpenAPI and runtime route sets match; documentation
  contract verification passed.
- Source migration coverage verifies both sanctioned-load constraints and the
  upgrade/rollback guard for independent charger inventory. The targeted
  PostgreSQL hub-assignment lifecycle remains pending because no explicitly
  disposable `TEST_DATABASE_URL` is configured.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed. Stateful PostgreSQL lifecycle verification is
  intentionally deferred by the current workstream decision; no stateful
  result is claimed.
- Revision `6930189` was built from a clean worktree and rehosted without a
  new migration. The running systemd process matches the installed binary
  SHA-256 `82781f61691cd93061f5e0d866ee63ce109298d053e74060d4242c2c8a817efe`
  and embeds revision `693018920d5600592a5b3a8409ce2e7790e25f80`
  with `vcs.modified=false`. The service is active and enabled; loopback and
  public liveness/readiness passed; the live OpenAPI contains 152 operations;
  the live CPO user-group member detail projection and `usergroup_assigned`
  response fields are present; and the post-start warning scan passed.
  The PowerShell documentation verifier remains unavailable on this Ubuntu
  host.
- Revision `a76d6ae` was built from a clean worktree and rehosted after
  migration thirty-one was applied. The installed binary SHA-256 is
  `0a3d397464dae13ef15b090225b4ca38fb1b4dfff946bf0de7d77cb9a5d3ebc0` and
  embeds revision `a76d6ae09dde7727661238f55e1b2ff5007394d0` with
  `vcs.modified=false`. The enabled service is active with zero restarts;
  local/public health, Swagger, raw OpenAPI, the 160-operation contract, and
  protected GST/settings/SuperAdmin route checks passed. The bounded SSE
  shutdown deadline occurred during stop and recovered; current systemd state
  is `Result=success`.
- Revision `e5fd599` was built from a clean worktree and rehosted after
  migration thirty was applied. The installed binary SHA-256 is
  `b6c251b9a65343db5eedbff3fee293678f4ac51cf401975b1d77380b1e47ef84` and
  embeds revision `e5fd599790b4fd9983ba055fc03b10637c2ad674` with
  `vcs.modified=false`. The enabled service is active with zero restarts;
  local/public health, Swagger, raw OpenAPI, the 160-operation contract, and
  protected settings/SuperAdmin route checks passed. The bounded SSE shutdown
  deadline occurred during stop and recovered; current systemd state is
  `Result=success`.
- Revision `2550cf7` was built from a clean worktree and rehosted after
  migration twenty-nine was confirmed current. The installed binary SHA-256
  is `5ebd7181ecae90a27787791c7c6f9786a3150fa968b3eb1d57bafa910c2418fa` and
  embeds revision `2550cf79fa6a9b84f3e30b0dca4101b8f0659574` with
  `vcs.modified=false`. The enabled service is active; local/public health,
  Swagger, raw OpenAPI, the 157-operation contract, and protected-route checks
  passed. The stop phase encountered the documented bounded SSE shutdown
  deadline and recovered; current systemd state is `Result=success` with
  `NRestarts=0`.
- Revision `f7e7227` was built from a clean worktree and rehosted after a
  mode-0600 PostgreSQL rollback dump and migration twenty-eight. The running
  systemd process matches the installed binary SHA-256
  `ab221e733317a832ea6b5bac60f5dcdc99c5a41d1f71d8edc2153b1c3161e957` and
  embeds revision `f7e722765809d0126b1ad8e84ba4ebb88e65f1d2` with
  `vcs.modified=false`. The service is active and enabled; loopback/public
  liveness/readiness, Swagger, raw OpenAPI, the 157-operation live contract,
  migration 28, protected charging routes, and the post-start warning scan
  passed. HAL provider credentials remain intentionally unset, so end-to-end
  virtual-charger acceptance is not claimed.
- Revision `79683f0` was built cleanly and rehosted without a new migration.
  The installed binary, loopback-only listener, local/public liveness and
  readiness, live 137-operation Swagger/OpenAPI with grouped CPO operations,
  protected CPO customer-list route, and post-start journal passed.
- Revision `9ccdff2` was built cleanly and rehosted after confirming migration
  twenty-seven and its replacement status constraints were already applied.
  The installed binary, loopback-only listener, local/public liveness and
  readiness, live 132-operation Swagger/OpenAPI, status and hub-publication
  operations, and post-start journal passed. The charger-create defaults now
  use the migration-27 values (`INACTIVE` charger, `ACTIVE` connector), so new
  inventory satisfies the live database constraint.
- Revision `782dd7b` was built cleanly and rehosted after a validated mode-0600
  rollback dump and migration twenty-six. The installed identity, loopback-only
  listener, loopback/public readiness, live 129-operation Swagger/OpenAPI,
  request-ID header, protected CPO routes, nullable charger hub assignment,
  required workers, and absence of startup errors or panics passed. Charger
  vendor and model persistence columns are nullable; charger create/update and
  customer network projections now preserve omitted metadata as null. Charger
  and customer-network part-time fields use `twenty_four_seven_open_status`.
  User App resources are served under `/api/v1/app`, while credentials and
  sessions remain under `/api/v1/app/auth`; the retired resource URLs return
  `404`. Charger-level `total_capacity`, connector current, and connector
  voltage fields are removed; connector-level capacity remains part of the
  connector contract.
- Revision `396bae5` was built cleanly and rehosted after a validated
  mode-0600 rollback dump and migration fourteen. The installed identity,
  loopback-only listener, loopback/public readiness, live 110-operation
  Swagger/OpenAPI, zero-restart service state, new authority columns and
  announcement/notification tables, and absence of startup errors or panics
  passed.
- `git diff --check` passed.
- Revision `9b508ef` was built cleanly and rehosted after a validated
  mode-0600 rollback dump and migration thirteen. The installed identity,
  loopback-only listener, loopback/public readiness, live 87-operation
  Swagger/OpenAPI, zero-restart service state, preserved feature-key rows in
  `retired_commercial`, and absence of startup errors or panics passed.
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
- The superseded global-customer lifecycle previously passed PostgreSQL tests.
  Migration twenty and the current CPO-local lifecycle have source/static/full
  Go coverage but still require an explicitly disposable `TEST_DATABASE_URL`.
- The customer-authentication migration rolled down, up, and idempotently up
  in PostgreSQL 17.
- Customer login OTP, encrypted access validation, `me`, refresh rotation and
  reuse revocation, customer-scoped session listing/revocation/logout, and
  password reset/change are implemented on dedicated CPO-local auth tables.
  Their updated lifecycle compiled but was not executed against PostgreSQL in
  this slice because no disposable `TEST_DATABASE_URL` was set.
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

- `users` represent administrative login identities for platform and CPO staff.
- `platform_admins` explicitly grant platform-superadmin authority.
- `cpos` represent tenant/customer organizations.
- `cpo_memberships` store a fixed role inside one CPO; current callable
  authority requires `ADMIN`.
- `customers` are CPO-local app-user accounts and own email, password, profile,
  verification, lockout, and login timestamps.
- Customer-auth outbox jobs are correlated to their owning CPO without
  pretending that the customer has an administrative `users` identity.

The full mapping from the supplied schema is recorded in `docs/SCHEMA.md`.

The same administrative identity may belong to multiple CPOs as staff. App
customers are separate: the same email can register independently under
multiple CPOs, with independent password/profile/session state. Only ADMIN
membership is currently accepted for CPO staff sessions; other stored role
values are dormant.

An administrative session selects exactly one platform or CPO scope. Protected
requests revalidate the durable session and current authority. Tenant context
comes from that principal rather than a request header. Tenant business routes
also verify that `X-CPO-App-ID` equals the current dummy or live ID for that
same principal; the app ID never grants authority or changes scope.

## HAL Boundary

The HAL remains a separate service and database. It is not embedded in this
repository. Source now contains the first CMS v1 consumer: an authenticated HAL
client, separate fact bearer receiver, durable start-intent/hold/command/fact
receipt/mapping/runtime records, and customer start/stop/polling routes.

This is not yet a verified complete operational integration. Disposable CMS and
HAL PostgreSQL lifecycle tests, bounded reconciliation, and the required
loopback HAL plus virtual OCPP charger acceptance remain outstanding. Customer
discovery must continue to treat CMS administrative status separately from
HAL-owned live runtime state.

## Known Limitations

- Password-recovery emails queued before recovery-ID delivery was implemented
  contain only the OTP and expiry and cannot complete reset. Users must request
  a new email; no database challenge lookup is an approved client workflow.
- A successful CPO-creation response proves its encrypted onboarding job
  committed, not that SMTP sent it. Operators must use primary-admin delivery
  status; only a newly created global identity receives a temporary password.
- Only the initial administrator profile and network/GST/tariff subset has
  handlers. Customer directory, charging, wallet mutation, recharge, payments,
  reporting, and most other domain tables remain without business APIs;
  published read-only customer network discovery, favorites, charger
  search/near-me, wallet balance/history, and informational tariff price reads
  are implemented.
- CPO staff invitation after the first admin and customer email/profile-change
  workflows are not implemented.
- Manual subscriptions are Superadmin-managed records; a CPO ADMIN has only a
  read-only current-subscription view. Feature keys and entitlement overrides
  are not defined. Platform invoices/payments and all provider or automatic
  lifecycle behavior remain unsupported, and CPO access is manual and
  independent.
- Automatic encryption-key rotation is not implemented; data must be
  re-encrypted before an old key is removed.
- SMTP delivery logic is implemented, worker-tested, and verified through one
  real Hostinger platform-login OTP delivery. The mailbox password remains only
  in the ignored deployment environment.
- No generated frontend SDK exists yet; consumers use the reviewed OpenAPI
  contract directly.
- Migration twenty's disposable PostgreSQL lifecycle coverage is intentionally
  deferred by decision. The live development deployment is current on
  migration twenty and the 113-operation contract; the two dormant feature-key
  tables are in
  `retired_commercial` while automatic lifecycle workers remain disabled.
