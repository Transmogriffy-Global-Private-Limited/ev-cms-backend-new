# Project State

## Current State

The repository began as an empty file scaffold. The implemented foundation now
provides:

- a compilable Go service;
- PostgreSQL connectivity and versioned migration execution;
- process-liveness and database-readiness endpoints;
- global identities;
- separate platform-superadmin records;
- CPO tenant organizations;
- fixed CPO-wide staff memberships;
- tenant-scoped customer relationships;
- user settings and tenant customer groups;
- hubs, chargers, connectors, favorites, and group access links;
- GST profiles and tariffs;
- wallets, wallet transactions, charging sessions, and wallet payments;
- platform and tenant audit logs;
- matching up and down migrations plus an explicit rollback operation.

The data structures exist, but no authentication, CPO management API, inventory
API, HAL integration, charging workflow, billing orchestration, payment
workflow, or reporting behavior is implemented yet.

## Verification

- Go formatting completed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- The embedded up migration created all 21 domain tables in a disposable local
  PostgreSQL 17 database.
- Reapplying up was idempotent and retained one migration version.
- The matching down migration removed all domain tables and retained only the
  migration ledger.

## Current Access Model

- `users` represent login identities.
- `platform_admins` explicitly grant platform-superadmin authority.
- `cpos` represent tenant/customer organizations.
- `cpo_memberships` grant a fixed role inside one CPO.
- `customers` represent a user's customer relationship with one CPO.

The full mapping from the supplied schema is recorded in `docs/SCHEMA.md`.

The same identity may belong to multiple CPOs. Its staff and customer
relationships remain distinct and tenant-scoped.

## HAL Boundary

The HAL remains a separate service and database. It is not embedded in this
repository. The integration contract has not been implemented yet.

## Known Limitations

- No authentication or authorization middleware is active.
- No initial superadmin bootstrap mechanism exists.
- CPO activation is represented in data but has no API.
- Domain tables have no repositories or handlers yet.
