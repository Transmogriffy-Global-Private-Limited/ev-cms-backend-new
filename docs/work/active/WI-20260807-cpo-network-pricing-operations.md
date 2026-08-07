# WI-20260807-cpo-network-pricing-operations

Status: In Progress
Owner: Abhranil Pal
Collaborators: Codex (guarded development-host deployment and verification)
Started: 2026-08-07
Last updated: 2026-08-07

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
- CPO connector create/update/response contracts use `total_capacity`; the
  database column remains `connector_total_capacity`.
- Charger projections may include `hub_name` when a same-CPO hub is assigned.

## Data and migration impact

No new migration. The production ledger remains at migration 27.

## Current state

Revision `79683f0` was rebuilt and rehosted on August 7, 2026 after OpenAPI
validation, full Go tests, and static analysis. The live service exposes the
137-operation OpenAPI document and keeps the new CPO customer routes protected.

## Verification

- OpenAPI/runtime parity, full Go tests, `go vet`, formatting, and whitespace
  checks passed.
- Active/enabled service, loopback listener, local/public liveness/readiness,
  public Swagger/OpenAPI, protected CPO customer-list route, migration ledger,
  and post-start journal were verified.
- The PowerShell documentation verifier is unavailable on this Ubuntu host.

## Handoff

The CPO implementation remains owned by Abhranil Pal. The current deployment
contains no schema change. Future changes to the listed CPO contracts must
retain the exact `total_capacity` API name and update the canonical OpenAPI,
human contract, consumer guidance, and verification together.

## Completion

In progress. This coordination item remains active while its owner continues
the claimed CPO network, pricing, and integration work.
