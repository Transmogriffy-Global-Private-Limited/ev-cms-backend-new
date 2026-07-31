# CPO Administrator and Initial Network Configuration Plan

Status: Implemented

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
- Hub create/list/get/update
- Charger/connector atomic create
- Bounded charger list
- Charger/connector get/update and dependency-safe charger delete
- GST create/list/get/update
- Tariff create/list/get/update
- Exact decimal tax/pricing values
- Server-generated charger, connector, OCPP-mapping, hub, GST, and tariff IDs
- Tenant scope on every query and related-record validation
- Transactional audit evidence
- OpenAPI, Swagger visibility, human contract, educational guide, tests, and
  project-memory updates

## Non-goals

- Tenant-side CPO organization profile
- App-user/customer changes
- Staff invitation, creation, or role management
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
- Dependent durable records prevent charger deletion.

## Implementation Slices

1. Reconcile branch conflicts and preserve the platform CPO implementation.
2. Remove tenant organization profile code and close authority to ADMIN.
3. Correct validation, generated IDs, exact tariff defaults, pagination, and
   dependency-safe delete behavior.
4. Add administrator identity profile.
5. Add focused/unit/PostgreSQL verification and route/OpenAPI parity.
6. Update educational, integration, API, architecture, project-state, and
   changelog documentation.
7. Commit the verified merge and align local `main` and `anubhab-work`.

## Acceptance Criteria

- Platform CPO routes from the authoritative branch still compile and pass.
- A CPO ADMIN can update their identity profile without changing email or role.
- No tenant organization profile route is registered or documented.
- Dormant roles cannot authenticate or invoke current CPO operations.
- All created records and references remain in the authenticated CPO.
- Required client identifiers are server-generated.
- Blank tariff currency persists as INR.
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

- Documentation verification passed at 68 operations.
- Focused protected-route, absent-organization-profile, and runtime/OpenAPI
  parity tests passed.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- PostgreSQL tests compile and are registered but did not execute locally
  because no disposable `TEST_DATABASE_URL` is configured. Do not promote this
  feature to `Verified` until that test runs against an explicitly disposable
  PostgreSQL database.

## Remaining Decisions

Before activating more staff roles, approve a complete staff lifecycle and
capability matrix. Before the HAL consumes `ocpp_identity`, approve the
authenticated, idempotent CMS/HAL mapping and reconciliation contract.
