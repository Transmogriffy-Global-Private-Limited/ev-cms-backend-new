# CPO Administrator and Initial Network Configuration Plan

Status: Verified

## Objective

Reconcile the existing CPO-side contribution into the authoritative branches
as the smallest correct tenant surface: one CPO ADMIN identity profile plus
tenant-scoped hubs, chargers/connectors, GST profiles, and tariffs.

## Dependencies

- Authentication and credential boundary
- Manual CPO provisioning and current app identity
- Initial CMS schema
- Platform-owned CPO activation and primary administrator recovery

## Scope

- ADMIN-only CPO session authority
- ADMIN identity profile read and full-name/phone update
- Read-only tenant-safe CPO organization details
- Tenant-scoped point lookup for a membership/customer-linked user UUID, with
  no directory or cross-CPO visibility
- Hub create/list/get/update
- Charger/connector atomic create
- Bounded charger list
- Charger/connector get/update and dependency-safe charger delete
- GST create/list/get/update
- Tariff create/list/get/update
- Optional paired tariff effective dates with database-enforced non-overlap for
  active tariffs in the same CPO/hub/charger/user-group scope
- Exact decimal tax/pricing values
- Server-generated charger, connector, OCPP-mapping, hub, GST, and tariff IDs
- Tenant scope on every query and related-record validation
- Transactional audit evidence
- OpenAPI, Swagger visibility, human contract, educational guide, tests, and
  project-memory updates

## Non-goals

- Tenant-side CPO organization mutation
- App-user/customer changes
- Staff invitation, creation, or role management
- CPO staff or customer directory/search
- Callable OWNER, OPERATOR, or VIEWER authority
- Hub, GST, or tariff deletion
- Connector add/remove after charger creation
- HAL calls, OCPP handshake, live status, commands, callbacks, or reconciliation
- Tenant realtime events
- Create-operation idempotency keys

## Ownership and Invariants

- Authenticated session supplies the trusted CPO ID.
- `X-CPO-App-ID` validates current application identity and never selects scope.
- Only active CPO `ADMIN` is callable.
- Dormant role enum values remain persistence capacity only.
- CPO organization fields remain platform-managed.
- Administrator profile fields belong to the global login identity.
- CMS OCPP identity is a mapping value; HAL remains protocol authority.
- Business mutation and audit evidence share one PostgreSQL transaction.
- Cross-CPO references are rejected as not found.
- Unknown and cross-CPO user UUIDs share the same `user_not_found` response.
- PostgreSQL is authoritative for concurrent active-tariff schedule overlap.
- Dependent durable records prevent charger deletion.

## Implementation Slices

1. Reconcile branch conflicts and preserve the platform CPO implementation.
2. Remove tenant organization mutation code and close authority to ADMIN.
3. Correct validation, generated IDs, exact tariff defaults, pagination, and
   dependency-safe delete behavior.
4. Add administrator identity profile and read-only organization projection.
5. Add focused/unit/PostgreSQL verification and route/OpenAPI parity.
6. Update educational, integration, API, architecture, project-state, and
   changelog documentation.
7. Commit the verified merge and align local `main` and `anubhab-work`.
8. Reconcile the tenant-safe CPO user point lookup and tariff effective-date
   contribution, then deploy migration fifteen before the corresponding binary.

## Acceptance Criteria

- Platform CPO routes from the authoritative branch still compile and pass.
- A CPO ADMIN can update their identity profile without changing email or role.
- The CPO ADMIN can read only its session-bound organization projection.
- No tenant organization mutation route is registered or documented.
- Dormant roles cannot authenticate or invoke current CPO operations.
- All created records and references remain in the authenticated CPO.
- User point lookup returns only identities linked to the authenticated CPO and
  never becomes a directory.
- Required client identifiers are server-generated.
- Blank tariff currency persists as INR.
- Tariff dates are both absent or form a valid half-open interval, and
  overlapping active periods in the same scope return
  `tariff_schedule_conflict` under concurrent writes.
- CPO callers cannot write charger or connector runtime status; those fields
  remain read-only HAL projections.
- Charger listing is bounded and cursor-recoverable.
- A referenced charger cannot be deleted and returns `charger_in_use`.
- Runtime routes and OpenAPI agree exactly.
- No HAL behavior is implied or invoked.

## Verification

- `go test ./src/auth ./src/integrations ./src/cpo -count=1`
- PostgreSQL `TestCPOAdminProfileAndNetworkConfigurationWithPostgreSQL` when
  `TEST_DATABASE_URL` points to an explicitly disposable database
- `go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1`
- `.\scripts\verify-docs.ps1`
- `go test ./...`
- `go vet ./...`
- `git diff --check`
- branch graph and merge-parent inspection

Completed evidence:

- Documentation verification and runtime/OpenAPI parity passed at 69
  operations.
- Focused protected-route, tenant-safe organization projection,
  absent-mutable-organization-profile, and runtime/OpenAPI parity tests passed.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- The complete PostgreSQL lifecycle passed against an explicitly created and
  removed disposable database. It covers organization/profile reads and
  updates, hub/charger/connector/GST/tariff writes, generated identifiers,
  ADMIN-only enforcement, audit evidence, and dependency-safe charger delete.
- PostgreSQL verification exposed and fixed explicit `open_24_hours` and
  `price_per_kwh` GORM mappings plus SQLSTATE `23001` handling for the
  documented `charger_in_use` conflict. Focused regression tests cover these
  database compatibility boundaries.
- Revision `bb555b9` and migration fifteen are deployed. The migration live
  preflight found no existing tariff rows, installed `btree_gist`, both date
  columns, both constraints, and both indexes; the 111-operation contract and
  protected user lookup passed hosted verification. The disposable overlap
  lifecycle remains pending without an explicitly disposable database.

## Remaining Decisions

Before activating more staff roles, approve a complete staff lifecycle and
capability matrix. Before the HAL consumes `ocpp_identity`, approve the
authenticated, idempotent CMS/HAL mapping and reconciliation contract.
