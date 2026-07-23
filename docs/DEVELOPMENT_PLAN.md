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
- Fixed CPO-wide staff roles initially
- Loopback-only local development listener by default

## Permanent Engineering Invariants

- A CPO is a tenant organization, not a user role.
- Login identity is global; CPO staff membership and CPO customer membership are
  tenant-scoped.
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
activate, and suspend CPOs while CPO owners manage their own staff.

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

## Feature Registry

### Feature: Lean tenancy and access foundation

Status: Verified

Phase: Foundation

Objective:

Represent the minimum safe access model: platform superadmins, CPO tenant
organizations, fixed CPO-wide staff roles, and tenant-scoped customers.

Scope:

- Global login identities
- Platform-superadmin marker
- CPO organization and lifecycle status
- Fixed CPO membership roles: owner, admin, operator, viewer
- Tenant customer relationship
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
- A customer relationship is scoped to a specific CPO.
- Duplicate membership and customer relationships are rejected by PostgreSQL.
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

## Current Execution

Current phase:

- Phase 1: Foundation (verified)

Active feature:

- None

Current implementation slice:

- No active implementation slice

Last completed slice:

- Complete supplied CMS domain mapped and verified through PostgreSQL up/down

Next expected slice:

- Authentication and initial-superadmin bootstrap design

Blocked by:

- None

## Next Approved Work

1. Design the authentication and initial-superadmin bootstrap flow.
2. Add superadmin-controlled CPO provisioning and activation.
3. Add CPO owner staff invitation and membership management.

## Deferred Work

- Custom tenant roles and fine-grained permissions
- Per-hub staff scopes
- Subscription plans and feature matrices
- PostgreSQL row-level security
- Cross-CPO roaming
- Redis, NATS, and service decomposition

## Risks and Unresolved Decisions

- The authentication and first-superadmin bootstrap mechanism is not yet
  approved.
- Commercial subscription rules are unknown; CPO activation is intentionally a
  simple manual platform action for now.
- The exact CMS/HAL API contract will be defined with the charging-network and
  charging-lifecycle phases.

## Verification Strategy

Run focused unit tests first, then `go test ./...`, `go vet ./...`,
`git diff --check`, and `git status --short`. Database integration tests will be
added when the first repository operation is implemented. The initial migration
has been verified up, idempotently up again, and down against a disposable local
PostgreSQL 17 database.

## Completion Criteria

The CMS is complete only when tenant boundaries, network management, HAL
integration, charging recovery, exact billing, operational verification, and
current project documentation work together end to end.
