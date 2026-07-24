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

## Supplied Model Mapping

| Supplied concept | Implemented data |
|---|---|
| `Role` | `platform_admins` for superadmin authority and the fixed `role` on `cpo_memberships` for CPO staff |
| `User` | `users` |
| `UserSetting` | `user_settings` |
| `CPOProfile` | `cpos`; the CPO is now an organization/tenant rather than a one-to-one user profile |
| `UserGroup` | tenant-owned `user_groups` |
| App user | `customers`, linking a user identity to one CPO |
| `Hub` | tenant-owned `hubs` |
| `Charger` | tenant-owned `chargers`, with separate public `charger_id` and `ocpp_identity` |
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

## Important Data Corrections

- Tenant-owned tables carry `cpo_id`.
- Composite foreign keys prevent a CPO from relating its customer, group, hub,
  charger, connector, tariff, wallet, session, or payment to another CPO's row.
- Financial values use PostgreSQL `numeric` and Go exact decimals rather than
  floating point.
- Sessions retain the HAL-issued integer `transaction_id`, integer meter Wh,
  billing totals, tariff/tax snapshots, timestamps, status, and stop reason.
- The six-character public charger ID is separate from the OCPP identity.
- Connector number is unique per charger.
- Historical and financial records use restrictive deletion rules rather than
  destructive cascades.

## Migration Behavior

Service startup applies pending up migrations only. An explicit migration
command can apply all pending migrations or roll back only the latest migration.
The application never executes a down migration automatically.

The up migration, idempotent reapplication, and matching down migration were
verified against a disposable loopback-only PostgreSQL 17 database. The test
database and its local data directory were removed after verification.

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

## Customer Signup

The fourth migration adds `customer_signup_challenges`. Pending registrations
are CPO-scoped and contain an Argon2id password hash plus HMAC-protected OTP,
never plaintext credentials. Successful verification creates or reuses
`users`, then creates the tenant `customers` relationship and its `wallet` in
one transaction. The migration also admits `CUSTOMER_SIGNUP_OTP` in the
encrypted mail outbox.

## Customer Authentication

The fifth migration adds a nullable `auth_sessions.customer_id`, enforced
against the session CPO through a composite foreign key. The session context
constraint permits exactly:

- `PLATFORM` with no CPO, role, or customer;
- `CPO` with a CPO and fixed staff role but no customer;
- `CUSTOMER` with a CPO and customer but no staff role.

It also admits CPO-bound customer login/password-reset challenges and their
encrypted mail templates. The same refresh-token lineage remains shared while
customer authorization is revalidated through the tenant customer record.

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

## Platform Subscriptions and Entitlements

The seventh migration adds:

- `subscription_plans`, stable commercial catalog identities;
- `subscription_plan_versions`, exact versioned currency/minor-unit price,
  billing interval, and trial terms;
- `subscription_plan_entitlements`, structured feature flags, limits, and
  non-secret JSON configuration owned by one version;
- `cpo_subscriptions`, the optional current and historical CPO commercial
  relationship;
- `cpo_subscription_history`, immutable, reasoned, idempotent lifecycle facts;
- `cpo_entitlement_overrides`, explicit reasoned CPO exceptions with optional
  expiry.

A partial unique index permits at most one `TRIAL`, `ACTIVE`, `PAUSED`, or
`PAST_DUE` subscription per CPO while allowing later assignments after a
terminal `CANCELLED` or `EXPIRED` record. CPO creation has no subscription
foreign key and remains valid without any subscription row.

Publishing a plan version records publisher/time. Database triggers reject
later update or deletion of that published version and its entitlement
snapshot. Scheduled plan changes and period-end cancellation retain their
operator identity so the lifecycle worker can write accountable history,
audit, event, and mail records when the boundary becomes due.

The migration also admits `CPO_SUBSCRIPTION_CHANGED` to the encrypted mail
outbox allowlist. Subscription mail payloads contain CPO/plan/status metadata,
never provider credentials or platform payment secrets.

## Provider-Neutral Platform Billing

The eighth migration adds one optional CPO billing account, exact platform
invoices and ordered lines, provider-neutral payments, and immutable payment
allocations. Actor-scoped idempotency keys protect invoice and payment
commands. External references are unique when supplied.

Invoice/payment CPO and currency equality, allocation bounds, and aggregate
paid/due transitions are enforced inside locked application transactions.
Database checks enforce exact minor-unit amount identities. After issue,
triggers reject changes to invoice commercial terms and lines while permitting
status and paid/due transitions.

Payment voiding retains allocation history but reverses its invoice-balance
effects atomically. A durable worker derives overdue status from due time and
remaining balance. These records describe what a CPO owes TransEV for the
platform; tenant Razorpay credentials and charging-customer payments remain
outside these tables.

The migration also admits encrypted `CPO_PLATFORM_INVOICE_ISSUED` mail work to
the explicit billing-account address.
