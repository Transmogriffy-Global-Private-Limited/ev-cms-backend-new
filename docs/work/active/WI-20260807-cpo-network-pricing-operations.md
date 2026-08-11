# WI-20260807-cpo-network-pricing-operations

Status: In Progress
Owner: Abhranil Pal
Collaborators: Codex (guarded development-host deployment and verification)
Started: 2026-08-07
Last updated: 2026-08-10

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — CPO administration and network configuration

Detailed-plan reference:

- `docs/plans/cpo-admin-network-configuration.md`

Issue/PR reference: None

## Outcome

Abhranil Pal owns the active CPO network, pricing, and integration-credential
backend work.

## Scope

- CPO hub, charger, GST, and tariff management
- CPO Razorpay integration-credential management
- CPO-owned static charger status and publication control
- Related CPO agent handoff, API contracts, and documentation

## Non-goals

- This record does not authorize a production deployment, DNS change, database
  mutation, credential disclosure, or external-provider change by itself.
- It does not assign ownership of platform CPO control, CPO/user
  authentication, customer-facing APIs, or HAL integration. Those remain owned
  by their respective work items.

## Claimed surfaces

- CPO network, pricing, and integration-credential routes, handlers, OpenAPI,
  and CPO agent handoff contracts
- `docs/CPO_ADMINISTRATION.md` and related CPO workflow/contract documentation

## Dependencies and blockers

- No database migration is required for the current CPO customer-directory,
  charger hub-name, and connector-capacity contract release.
- The disposable PostgreSQL lifecycle suite remains deferred until an explicitly
  selected `TEST_DATABASE_URL` is available.

## Contract impact

- CPO ADMIN customer list/detail reads are tenant-scoped and read-only.
- CPO connector create/update requests and response objects use
  `connector_total_capacity`, matching the persisted connector-capacity field.
- Charger projections may include `hub_name` when a same-CPO hub is assigned.

## Data and migration impact

This CPO slice introduced migration twenty-nine for nullable tariff metadata;
the shared development deployment also records migration twenty-eight for the
separate CMS/HAL charging vertical.

## Current state

Revision `2550cf7` was built from a clean worktree and rehosted on
August 11, 2026 after migration twenty-nine. The live service exposes the
157-operation OpenAPI document, and the CPO user-group membership contract now
includes tenant-scoped, idempotent assignment/removal, the `members` detail
projection, plus the `usergroup_assigned` customer projection. The CPO
`Connector` response schema now
documents the runtime `connector_total_capacity` projection. Scoped tariff
and user-group routes are protected by the same tenant authorization boundary.

## Verification

- The current OpenAPI request example, route-contract check, `go test ./...`,
  `go vet ./...`, and `git diff --check` passed after the capacity-name
  reconciliation.
- OpenAPI/runtime route-contract verification passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- Revision `2550cf7` is active under `evcmsnew-dev.service`; the running
  process matches the installed binary and the expected VCS revision.
- Loopback/public liveness and readiness passed.
- The live OpenAPI exposes 157 operations, and the live CPO user-group detail
  `members` projection plus `usergroup_assigned` response field are present.
- The live CPO `Connector` response schema exposes `connector_total_capacity`.
- The current systemd state is active/enabled with zero restarts after the
  bounded SSE shutdown deadline recovered during rehost.
- Migration twenty-nine is applied; its nullable tariff metadata is null-safe
  for existing rows and omitted request fields.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

## Handoff

The CPO implementation remains owned by Abhranil Pal. The current deployment
contains no schema change. Connector create/update request payloads and
response objects use `connector_total_capacity`. Update the canonical OpenAPI,
human contract, consumer guidance, and verification together.

## Completion

In progress. This coordination item remains active while its owner continues
the claimed CPO network, pricing, and integration work.
