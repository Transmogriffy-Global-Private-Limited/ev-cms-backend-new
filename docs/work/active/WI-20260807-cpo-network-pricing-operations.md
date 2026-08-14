# WI-20260807-cpo-network-pricing-operations

Status: In Progress
Owner: Abhranil Pal
Collaborators: Codex (guarded development-host deployment and verification)
Started: 2026-08-07
Last updated: 2026-08-13

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

### Coordination note (2026-08-13)

The user-authorized tariff-targeting correction is being implemented under
`../archive/WI-20260813-tariff-targeting-visibility-sweep.md`. It overlaps the CPO tariff
schema, scoped routes, and contracts while preserving Abhranil Pal's ownership
of the broader CPO network/pricing capability.

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
- CPO `ADMIN` charging-session list/detail reads are tenant-scoped, bounded by
  `(created_at, id)`, and documented in the administrative API contract.

## Data and migration impact

This CPO slice introduced migration twenty-nine for nullable tariff metadata
and migration thirty-two for required hub state; the shared development
deployment also records migration twenty-eight for the separate CMS/HAL
charging vertical.

## Current state

The current shared development deployment is runtime source revision `11c4c23`
with migrations through thirty-nine applied and 180 OpenAPI
operations. The single-target tariff correction and charger customer visibility
are enforced across CPO publication and User App discovery, detail, pricing,
and favorites. The HAL runtime model
mapping now explicitly targets the singular
`hal_charger_runtime` and `hal_connector_runtime` tables; no migration was
needed for this correction. State-aware GST-to-hub assignment validation is
also active.
- CPO charger list/detail responses additionally expose optional committed live
  connection/connector projections; list reads use one bounded batch lookup.

Earlier revision `3ca2c35` was built from a clean worktree and rehosted on
August 11, 2026 after migration thirty-two. The live service exposes the
162-operation OpenAPI document, and the CPO user-group membership contract now
includes tenant-scoped, idempotent assignment/removal, the `members` detail
projection, plus the `usergroup_assigned` customer projection. The CPO
`Connector` response schema now
documents the runtime `connector_total_capacity` projection. Scoped tariff
and user-group routes are protected by the same tenant authorization boundary.
The local merge reconciliation also completes the CPO hub `state` contract:
create requires it, update accepts it, and hub list/detail responses return it.
The hub `state` field is persisted and included in the live CPO hub contracts.

## Verification

- `go test -p 1 ./src/cpo ./src/customerauth -count=1` and
  `./scripts/verify-docs.ps1` passed for the merged hub-state source and
  contract repair. The current tariff-release deployment verification is
  recorded in the archived tariff work item and project state.
- The current OpenAPI request example, route-contract check, `go test ./...`,
  `go vet ./...`, and `git diff --check` passed after the capacity-name
  reconciliation.
- OpenAPI/runtime route-contract verification passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- Earlier revision `3ca2c35` was active under `evcmsnew-dev.service`; the installed
  binary SHA-256 is
  `1771f4bee02ed7f5d270f9feacdad69ecb4c3de435103ea5d7c97e176ab9da12`, and
  rollback artifacts are `builds/evcmsnew.pre-3ca2c35` and
  `/tmp/devevcmsnewdb-pre-3ca2c35.dump`.
- Migration thirty-two is applied; loopback/public liveness and readiness,
  Swagger, raw OpenAPI, the live 162-operation contract, and protected-route
  checks passed. The post-rehost journal has no error, panic, or fatal records.
- The later shared deployment is revision `3f3a952`: migration thirty-three is
  applied and the current live OpenAPI surface has 172 operations. Service,
  public/loopback health, Swagger, and raw OpenAPI remain healthy.
- Revision `2e8fdb3` is now active after migration thirty-four added nullable
  tariff assignment metadata; the live service remains healthy with 172
  OpenAPI operations.
- Earlier revision `e831b32` was active after migration thirty-five added same-CPO
  GST-to-hub assignment; the live service exposes 176 OpenAPI operations and
  all four GST-hub route auth boundaries pass.
- Revision `0d50c09` is now active after the HAL runtime model correction. The
  clean installed binary SHA-256 is
  `e3790854e68f7a3996d50a552e2f15ef6a95f644184e10389a95d513d64b24bf`; service,
  Caddy, health, Swagger/OpenAPI, table, and post-rehost journal checks passed.
- Earlier revision `d368903` deployment evidence established the prior
  161-operation contract, CPO user-group projections, connector capacity
  contract, and migration twenty-nine behavior before the current release.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

## Handoff

The CPO implementation remains owned by Abhranil Pal. The current deployment
contains migrations through thirty-nine, the single-target tariff correction,
and the charger customer-visibility gate
plus the HAL runtime table mapping
correction. Connector create/update request payloads and
response objects use `connector_total_capacity`. Update the canonical OpenAPI,
human contract, consumer guidance, and verification together.

## Completion

In progress. This coordination item remains active while its owner continues
the claimed CPO network, pricing, and integration work.
