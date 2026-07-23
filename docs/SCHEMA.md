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
