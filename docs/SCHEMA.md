# CMS Schema

## Purpose

The initial migration preserves every business area from the supplied CMS
schema while correcting the ownership model so CPO data is tenant-scoped.

Migration files:

- `db/migrations/000001_cms_schema.up.sql`
- `db/migrations/000001_cms_schema.down.sql`
- `db/migrations/000002_auth_credentials.up.sql`
- `db/migrations/000002_auth_credentials.down.sql`
- `db/migrations/000003_cpo_provisioning.up.sql`
- `db/migrations/000003_cpo_provisioning.down.sql`
- `db/migrations/000004_customer_signup.up.sql`
- `db/migrations/000004_customer_signup.down.sql`
- `db/migrations/000005_customer_authentication.up.sql`
- `db/migrations/000005_customer_authentication.down.sql`
- `db/migrations/000006_platform_operations.up.sql`
- `db/migrations/000006_platform_operations.down.sql`
- `db/migrations/000007_subscriptions.up.sql`
- `db/migrations/000007_subscriptions.down.sql`
- `db/migrations/000008_platform_billing.up.sql`
- `db/migrations/000008_platform_billing.down.sql`
- `db/migrations/000009_retire_subscriptions_and_platform_billing.up.sql`
- `db/migrations/000009_retire_subscriptions_and_platform_billing.down.sql`
- `db/migrations/000010_cpo_superadmin_dependency.up.sql`
- `db/migrations/000010_cpo_superadmin_dependency.down.sql`
- `db/migrations/000011_cpo_required_registration_fields.up.sql`
- `db/migrations/000011_cpo_required_registration_fields.down.sql`
- `db/migrations/000012_restore_manual_subscriptions.up.sql`
- `db/migrations/000012_restore_manual_subscriptions.down.sql`
- `db/migrations/000013_retire_dormant_subscription_entitlements.up.sql`
- `db/migrations/000013_retire_dormant_subscription_entitlements.down.sql`
- `db/migrations/000014_complete_superadmin_surface.up.sql`
- `db/migrations/000014_complete_superadmin_surface.down.sql`
- `db/migrations/000015_tariff_effective_dates.up.sql`
- `db/migrations/000015_tariff_effective_dates.down.sql`
- `db/migrations/000028_cms_hal_charging_vertical.up.sql`
- `db/migrations/000028_cms_hal_charging_vertical.down.sql`
- `db/migrations/000029_add_tariff_fields.up.sql`
- `db/migrations/000029_add_tariff_fields.down.sql`
- `db/migrations/000030_add_settings_table.up.sql`
- `db/migrations/000030_add_settings_table.down.sql`
- `db/migrations/000031_add_state_to_gsts.up.sql`
- `db/migrations/000031_add_state_to_gsts.down.sql`
- `db/migrations/000032_add_state_to_hubs.up.sql`
- `db/migrations/000032_add_state_to_hubs.down.sql`
- `db/migrations/000033_operational_realtime_events.up.sql`
- `db/migrations/000033_operational_realtime_events.down.sql`
- `db/migrations/000034_add_assigned_to_to_tariffs.up.sql`
- `db/migrations/000034_add_assigned_to_to_tariffs.down.sql`
- `db/migrations/000035_add_gst_id_to_hubs.up.sql`
- `db/migrations/000035_add_gst_id_to_hubs.down.sql`
- `db/migrations/000036_add_customer_visibility_to_chargers.up.sql`
- `db/migrations/000036_add_customer_visibility_to_chargers.down.sql`

## Supplied Model Mapping

| Supplied concept | Implemented data |
|---|---|
| `Role` | `platform_admins` for superadmin authority and the fixed `role` on `cpo_memberships` for CPO staff |
| `User` | `users` |
| `UserSetting` | `user_settings` |
| `CPOProfile` | `cpos`; the CPO is now an organization/tenant rather than a one-to-one user profile |
| `UserGroup` | tenant-owned `user_groups` |
| App user | `customers`, a credential-owning account local to one CPO |
| `Hub` | tenant-owned `hubs`, including non-negative sanctioned load in kW (`0` when not recorded) |
| `Charger` | tenant-owned `chargers`, with optional same-CPO hub assignment and a `customer_visibility` publication gate; newly created rows assign the public `charger_id` to the compatibility `ocpp_identity` too |
| `Connector` | tenant-owned `connectors` |
| Group-to-hub access | `user_group_hubs` |
| Group-to-charger access | `user_group_chargers` |
| Favorite hubs | `customer_favorite_hubs` |
| Favorite chargers | `customer_favorite_chargers` |
| `GST` | tenant-owned `gsts` |
| `Tariff` | tenant-owned `tariffs` |
| `Wallet` | one tenant-owned `wallet` per customer |
| `WalletTransaction` | `wallet_transactions` |
| `ChargingSession` | `charging_sessions` |
| `Payment` | `payments` |
| `AuditLog` | `audit_logs`; `cpo_id` is nullable for platform events |
| CMS HAL charging consumer | `charging_start_intents`, `wallet_holds`, `hal_command_records`, `hal_fact_receipts`, `hal_charger_mappings`, `hal_charger_runtime`, and `hal_connector_runtime` |
| Durable operational notification | `operational_events`, scoped to a CPO and optionally a customer |

## Important Data Corrections

- Tenant-owned tables carry `cpo_id`.
- Composite foreign keys prevent a CPO from relating its customer, group, hub,
  charger, connector, tariff, wallet, session, or payment to another CPO's row.
- Financial values use PostgreSQL `numeric` and Go exact decimals rather than
  floating point.
- Sessions retain the HAL-issued integer `transaction_id`, integer meter Wh,
  billing totals, tariff/tax snapshots, timestamps, status, and stop reason.
- The CMS charger UUID is separate from the six-character public charger ID.
  New charger creation assigns that public value to `ocpp_identity` as the
  compatibility mapping value; older rows retain their stored identity.
- A charger can be created without a hub. When assigned, its nullable composite
  `(cpo_id, hub_id)` foreign key ensures the hub belongs to the same CPO.
- Connector number is unique per charger.
- Historical and financial records use restrictive deletion rules rather than
  destructive cascades.
- Migration twenty-eight extends the legacy session projection with exact HAL
  transaction correlation and live meter fields. It adds durable CMS business
  intent/hold/receipt/projection tables; the HAL database remains separate.
- Migration twenty-nine adds nullable `tariff_type`, `price_type`, and `units`
  columns. Existing tariffs remain valid with null metadata, and omitted API
  fields do not write empty enum values.
- Migration thirty adds the tenant-owned `settings` table with one unique row
  per CPO, optional invoice logo path, and optional invoice note. It references
  `cpos(id)` and uses the existing `gen_random_uuid()` database function.
- Migration thirty-one adds nullable GST state and makes legacy GST-rate
  columns nullable. API creation still requires a non-empty state and all
  three rate values; the nullable columns preserve historical database rows.
- Migration thirty-two adds the required hub `state` persistence column with a
  compatibility default for existing records; the API enforces non-empty state
  values for new and updated hubs.
- Migration thirty-three adds the durable `operational_events` ledger. Its
  CPO/customer foreign keys preserve tenant ownership, and cursor/retention
  indexes support scoped REST replay and SSE recovery without making realtime
  delivery the source of truth.
- Migration thirty-four added `tariffs.assigned_to` using the PostgreSQL
  `tariff_assignment_type` enum. Migration thirty-seven normalizes legacy
  composite scope rows with `usergroup > charger > hub` precedence, makes
  `assigned_to` non-null, makes all three target IDs nullable, and enforces one
  target plus a matching assignment. The charger target foreign key is
  `(cpo_id, charger_id)` to allow independent same-CPO chargers without
  inventing hub context. Its guarded rollback refuses non-hub targets rather
  than deleting data or fabricating a hub relationship.
- Migration thirty-five adds nullable `hubs.gst_id` with a same-CPO foreign key
  to `gsts`. The CPO hub GST assignment APIs own assign, read, replace, and
  unassign behavior; no cross-CPO GST can be attached.
- `HALChargerRuntime` and `HALConnectorRuntime` explicitly map through GORM
  `TableName` methods to the singular `hal_charger_runtime` and
  `hal_connector_runtime` tables created by migration twenty-eight.

## Migration Behavior

Service startup applies pending up migrations only. An explicit migration
command can apply all pending migrations or roll back only the latest migration.
The application never executes a down migration automatically.

Migrations through version ten, idempotent reapplication, and matching rollback
were verified against a disposable loopback-only PostgreSQL 17 database. The
test database and its local data directory were removed after verification.
Migration eleven has source and compilation coverage but has not run against a
disposable PostgreSQL database in the current slice because `TEST_DATABASE_URL`
is not configured.

## Authentication and Credential Tables

The second migration adds:

- user lockout, MFA, password-change, and last-login state;
- single-use `auth_challenges`;
- scope-bound `auth_sessions`;
- hashed, rotating `auth_refresh_tokens`;
- encrypted durable `mail_outbox` jobs;
- database-backed `auth_rate_limits`;
- encrypted tenant-owned `cpo_integrations`.

Session constraints distinguish platform sessions from CPO sessions. CPO
sessions require a CPO and fixed role; platform sessions permit neither.
Integration credentials are restricted to the supported provider allowlist and
are never stored in plaintext.

## CPO Provisioning and App Identity

The third migration adds:

- `users.must_change_password` for temporary-password onboarding;
- unique `cpos.app_id`;
- `cpos.app_id_mode`, constrained to `DUMMY` or `LIVE`;
- `cpos.app_id_updated_at`;
- encrypted mail-outbox templates for CPO onboarding, membership assignment,
  and password-change reminders.

Existing CPOs are backfilled with unique dummy IDs. Dummy IDs use the reserved
`cpo_dummy_` prefix. Live IDs cannot use that prefix. CPO lifecycle remains
independent from app-ID mode, and no subscription table or subscription
foreign key is introduced.

## Required CPO Registration Identity

Migration eleven strengthens the platform-owned CPO registration record:

- `gstin` becomes `NOT NULL` and retains its 15-character uppercase format
  check;
- `address`, `city`, `state`, and `pincode` lose their empty-string defaults
  and gain nonblank checks;
- the existing `uq_cpos_slug_normalized` index remains the authoritative
  case-insensitive slug uniqueness guard;
- the existing `uq_cpos_gstin_normalized` index covers every row once GSTIN is
  non-null and remains the authoritative case-insensitive GSTIN uniqueness
  guard.

The migration performs a preflight and stops if any existing CPO lacks these
values. It deliberately does not invent a GSTIN or address. Operators must
correct incomplete records from an authoritative source before applying it.

## Customer Signup

The fourth migration adds `customer_signup_challenges`. Pending registrations
are CPO-scoped and contain an Argon2id password hash plus HMAC-protected OTP,
never plaintext credentials. Under migration twenty, successful verification
creates the tenant-local `customers` account and its `wallet` in one
transaction without touching `users`. The migration also admits `CUSTOMER_SIGNUP_OTP` in the
encrypted mail outbox.

## Historical Customer Authentication Foundation

The fifth migration adds a nullable `auth_sessions.customer_id`, enforced
against the session CPO through a composite foreign key. The session context
constraint permits exactly:

- `PLATFORM` with no CPO, role, or customer;
- `CPO` with a CPO and the currently callable `ADMIN` role but no customer;
- `CUSTOMER` with a CPO and customer but no staff role.

It also admitted CPO-bound customer login/password-reset challenges and their
encrypted mail templates. Migration twenty supersedes the global-user parts of
this design while retaining the mail templates and public API behavior.

## CPO-Local Customer Accounts

Migration twenty is deliberately guarded by an empty-`customers` preflight;
the product decision was made before customer data existed, and the migration
refuses to guess how historical credentials should be split.

It removes `customers.user_id`, moves email/password/profile/verification/
lockout/login state onto `customers`, and enforces normalized email uniqueness
per CPO. It creates:

- `customer_auth_challenges`, with CPO/customer composite ownership, HMAC OTP,
  expiry, attempts, resend cooldown, and terminal timestamps;
- `customer_auth_sessions`, with CPO/customer composite ownership, token
  version, client metadata, expiry, and revocation state;
- `customer_auth_refresh_tokens`, with hashed one-time tokens, rotation
  lineage, reuse detection, expiry, and revocation state.

The general `auth_challenges`, `auth_sessions`, and `auth_refresh_tokens` tables
return to administrative `PLATFORM`/`CPO` use only. The customer JWT subject is
the CPO-local customer UUID. A rollback also refuses to run while customer rows
exist because independent customer credentials cannot be reconstructed as one
global identity without an explicit data-migration policy.

## Platform Operations, Workers, and Realtime

The sixth migration adds:

- `platform_events`, an append-only, monotonically ordered, retention-bounded
  event log used for superadmin UI invalidation, replay, and SSE delivery; and
- `worker_instances`, durable process-instance heartbeats used to derive
  `HEALTHY`, `DEGRADED`, `DISABLED`, or `STALE` operational state.

`platform_events.id` is the canonical replay and deduplication cursor. Events
are committed in the same database transaction as the state change they
announce. Their JSON payload is metadata only and must never contain passwords,
OTPs, tokens, integration credentials, or decrypted mail bodies. Expired rows
are removed by the platform-maintenance worker; authoritative business state
remains in the owning tables and must be re-fetched after an event.

Worker identity is the unique `(worker_name, instance_key)` pair. Required
worker rows with a non-healthy reported status or a heartbeat older than
`PLATFORM_WORKER_STALE_AFTER` make `/health/ready` unavailable. A process that
has not registered a worker row is not inferred from this table; startup owns
registration by sending an immediate heartbeat.

## Retired Platform Billing and Restored Manual Subscriptions

Migrations seven and eight historically created subscription, entitlement,
platform-invoice, and platform-payment tables. They remain unchanged because
both migrations reached the development VPS.

The ninth migration implements the approved product reversal:

- it refuses to proceed while a related subscription/invoice mail job is
  `PENDING` or `PROCESSING`;
- it removes commercial immutability triggers and moves all eleven prototype
  tables from `public` into `retired_commercial`;
- it preserves every row rather than dropping commercial history;
- it marks `subscription-lifecycle` and `billing-maintenance` worker records
  non-required and `DISABLED`, preventing stale retired workers from degrading
  readiness;
- its down migration restores the tables, triggers, and worker requirements.

Migration twelve restored the six subscription/entitlement tables. Migration
thirteen returns `subscription_plan_entitlements` and
`cpo_entitlement_overrides` to `retired_commercial` because the product does
not yet define concrete feature gates. `subscription_plans`,
`subscription_plan_versions`, `cpo_subscriptions`, and
`cpo_subscription_history` remain active in `public`. Published plan/version
snapshots are protected by the restored immutability triggers. The five
platform-billing tables remain in `retired_commercial`.

The restored subscription records are manually managed by platform
superadmins. The migration does not re-enable `subscription-lifecycle` or
`billing-maintenance`; both remain non-required and `DISABLED`. Period dates
are records only, and CPO `ACTIVE`/`SUSPENDED` lifecycle state remains the only
platform-access control.

Migration nine was verified up and down against disposable loopback PostgreSQL
17, including pending-mail refusal, preserved-row archival/restoration, and
retired-worker disable/restore behavior.

## Tariff Effective Dates

Migration fifteen is additive and must run before a binary that reads or writes
`tariffs.start_date` or `tariffs.end_date`. A tariff is either open-ended (both
columns null) or uses a complete half-open effective interval. The migration
fails before adding its exclusion constraint if existing active tariffs already
overlap under the same `(cpo_id, hub_id, charger_id, user_group_id)` tuple;
operators must reconcile those records deliberately rather than receiving an
ambiguous partial migration. The `btree_gist`-backed constraint treats null
effective bounds as infinity and uses `IS NOT DISTINCT FROM` semantics for the
optional UUID target columns, so it remains safe under concurrent writes.

## Hub Sanctioned Load and Independent Chargers

Before initial deployment, the base CMS migration was reconciled so
`chargers.hub_id` is nullable. This permits independent charger inventory while
the composite foreign key still prevents any non-null assignment from crossing
CPO boundaries. Migration thirty-seven permits a charger-specific tariff for an
independent same-CPO charger because a tariff now has one charger target rather
than mandatory hub context.

Migration sixteen adds `hubs.sanction_load numeric(10,2) NOT NULL DEFAULT 0`,
`chk_hubs_sanction_load`, and drops the legacy `chargers.hub_id NOT NULL`
constraint for existing deployments. The service rejects negative values before
the write and maps the database constraint to the same field-level client error.
Its rollback refuses to restore `NOT NULL` while independent chargers exist.
The former redundant OCPP-identity index migration and split sanction-load
constraint migration were removed before deployment: migration one already owns
the global OCPP identity uniqueness invariant, and migration sixteen owns the
complete sanctioned-load invariant.

Migration twenty-one adds `hubs.customer_visible boolean NOT NULL DEFAULT false`.
The CPO ADMIN hub create/update API owns this publication switch. Customer
discovery reads only `true` hubs and attached same-CPO chargers; independent
chargers and unpublished hubs remain durable CMS inventory but are not exposed
to the User App.

## CPO Superadmin Dependency

Migration ten completes the durable state required by the CPO control-plane
workflow:

- `cpos.status_reason`, `status_changed_at`, and
  `status_changed_by_user_id` retain the current manual lifecycle decision;
- `cpo_memberships.is_primary_admin` designates one responsible membership,
  enforced by a partial unique index per CPO. Current application
  orchestration normalizes that membership to `ADMIN`; dormant fixed-role enum
  values remain schema capacity for a later staff-management feature;
- existing CPOs backfill the oldest eligible staff membership as primary,
  preferring active membership status; a CPO with no eligible membership
  remains explicitly without a primary until the recovery API establishes one;
- `mail_outbox.cpo_id` and `user_id` correlate safe job metadata with the CPO
  and recipient identity without exposing the encrypted payload; and
- the `CPO_ONBOARDING_RESENT` template supports credential-free recovery.

CPO state, primary-membership changes, audit evidence, durable platform events,
session revocation, and applicable mail jobs share application-owned PostgreSQL
transactions. The database enforces uniqueness and relationship validity but
does not hide lifecycle orchestration in triggers.

The migration-ten up and down paths were executed against a disposable
loopback-only PostgreSQL 17 database. The full CPO lifecycle test covered
creation correlation, search/cursor behavior, profile replacement, reasoned
idempotent lifecycle change, administrator replacement, targeted session
revocation, credential-free resend, and platform-session isolation.
