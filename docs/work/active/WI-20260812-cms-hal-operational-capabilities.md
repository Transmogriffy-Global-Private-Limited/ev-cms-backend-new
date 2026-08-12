# WI-20260812-cms-hal-operational-capabilities

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-08-12
Last updated: 2026-08-12

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — Charging lifecycle and HAL integration
Detailed-plan reference: `docs/integrations/ocpp-hal-boundary.md`
Issue/PR reference: None

## Outcome

Establish reusable CMS capabilities over HAL-derived operational truth and expose CPO, App, and Platform-safe live snapshots without direct HAL coupling.

## Scope

- HAL operations/reconciliation, shared fact ingress, CMS-projection live reads, inventory-owned pending mappings, CPO/App/Platform operational REST and SSE, durable operational-event publication, migration, OpenAPI, integration docs, and guarded development deployment.

## Non-goals

- HAL, legacy CMS, OCPP protocol, tariff/GST redesign, Superadmin charger control, and physical-charger acceptance.

## Claimed surfaces

- `src/halclient`, `src/halops`, `src/liveops`, `src/operationalrealtime`, `src/customerauth`, `src/cpo`, models/migrations, routes, OpenAPI, and HAL integration docs.
- User App full charger list, hub detail, single detail, and favorites; compact
  map locations remain explicitly out of scope.

## Dependencies and blockers

- Read-only HAL provider `21836e5d98967399d599d6afeca52fe1c375ec0d`.
- Disposable PostgreSQL and CMS-to-HAL-to-virtual-charge-point topology are required for full acceptance.

## Contract impact

- Adds CPO/App/Platform operational read and scoped SSE contracts. REST snapshots remain authoritative; business APIs use CMS capabilities rather than HAL transport.

## Data and migration impact

- Adds migration 33 for durable scoped operational notifications. It is applied
  on the development VPS after a validated rollback dump.

## Current state

- `halops` owns CMS mapping/command mechanics, exact-ID reconciliation, and fact ingress; `halclient` remains the wire adapter.
- `liveops` reads committed CMS projections and centralizes freshness/offline connector semantics.
- CPO/Platform snapshots and customer own-session/customer-safe charger detail use those capabilities.
- Full CustomerCharger list, hub-detail, single-detail, and favorite responses
  now use the same bounded `liveops.GetChargerDetails` overlay. Static CMS
  lifecycle remains a customer-safety gate; compact map markers remain only
  name and coordinates.
- The canonical CPO backend HAL operational capability manual is
  `docs/integrations/cpo-hal-operational-capability-manual.md`; it documents
  current CPO reads/SSE and the future command pattern without adding a CPO
  command endpoint.
- Durable operational events are produced with accepted newer fact projections and consumed through scoped REST cursor replay/SSE. Streams revalidate bearer sessions at heartbeat.
- Revision `27c01f3` is active under `evcmsnew-dev.service`; migration thirty-three,
  loopback/public health and readiness, Swagger, raw OpenAPI, the live
  172-operation contract, and HAL fact-ingress validation have been verified.

## Verification

- Focused User App and live-projection package tests pass after the batch
  overlay refactor. `go test ./...`, `go vet ./...`, the PowerShell
  documentation verifier, OpenAPI/runtime parity, and `git diff --check` pass
  for this merged source state.
- Focused route/OpenAPI verification, `go test ./...`, `go vet ./...`, Caddy
  validation, clean build, and `git diff --check` passed. Migration ledger,
  table/index, service, health, Swagger, raw OpenAPI, and HAL fact-ingress
  validation checks passed after rehost.
- The shared full-response User App overlay was rehosted without a migration;
  current service state has zero restarts, public/loopback health and docs are
  healthy, and no post-restart warning was recorded. The PowerShell verifier
  remains unavailable on this Ubuntu host because `pwsh` is not installed.
- Fact/mapping/SSE lifecycle behavior, reconciliation recovery, and dual-service
  E2E remain pending. The earlier development host lacked `pwsh`; the current
  Windows source checkout passed the PowerShell documentation verifier.

## Handoff

- Do not mark complete until lifecycle/mapping/fact/SSE behavior and topology acceptance are proven.

## Completion

Not complete.
