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

No new migration. The production ledger remains at migration 27.

## Current state

Revision `9b57f20` was built from a clean worktree and rehosted on
August 10, 2026 without a new migration. The live service exposes the
137-operation OpenAPI document, and the CPO `Connector` response schema now
documents the runtime `connector_total_capacity` projection.

## Verification

- The current OpenAPI request example, route-contract check, `go test ./...`,
  `go vet ./...`, and `git diff --check` passed after the capacity-name
  reconciliation.
- OpenAPI/runtime route-contract verification passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- Revision `9b57f20` is active under `evcmsnew-dev.service`; the running
  process matches the installed binary and the expected VCS revision.
- Loopback/public liveness and readiness passed.
- The live OpenAPI exposes 137 operations, and the live CPO `Connector`
  response schema exposes `connector_total_capacity`.
- The post-start fatal-error scan passed.
- No migration file changed from the previously deployed revision.
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
