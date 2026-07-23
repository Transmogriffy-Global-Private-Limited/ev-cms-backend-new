# CMS Schema

## Purpose

The initial migration preserves every business area from the supplied CMS
schema while correcting the ownership model so CPO data is tenant-scoped.

Migration files:

- `db/migrations/000001_cms_schema.up.sql`
- `db/migrations/000001_cms_schema.down.sql`

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
