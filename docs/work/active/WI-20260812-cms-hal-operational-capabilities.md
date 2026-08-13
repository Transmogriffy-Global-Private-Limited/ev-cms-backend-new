# WI-20260812-cms-hal-operational-capabilities

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-08-12
Last updated: 2026-08-13

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

- Adds migration 33 for durable scoped operational notifications, migration 34
  for nullable tariff assignment metadata, and migration 35 for same-CPO
  GST-to-hub assignment. All are applied on the
  development VPS after validated rollback dumps.

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
- The same canonical manual now provides the junior-oriented safe-extension,
  return-value, identifier, lifecycle, debugging, and verification reference;
  the CPO handoff is intentionally only its orientation and reading path.
- The current CPO integration repair keeps those capabilities intact while
  removing duplicate response declarations and create-path mapping delivery;
  new charger identity compatibility and migration-34 tariff metadata are
  source-aligned. The later serial-number HAL admission contract remains out
  of scope.
- Durable operational events are produced with accepted newer fact projections and consumed through scoped REST cursor replay/SSE. Streams revalidate bearer sessions at heartbeat.
- Revision `87b8727` is active under `evcmsnew-dev.service`; migrations
  thirty-three through thirty-five,
  loopback/public health and readiness, Swagger, raw OpenAPI, the live
  177-operation contract, singular HAL runtime table mappings, and GST-hub
  route boundaries have been verified. No migration was needed for the model
  correction or the User App history release.

## Verification

- Focused User App and live-projection package tests pass after the batch
  overlay refactor. `go test ./...`, `go vet ./...`, the PowerShell
  documentation verifier, OpenAPI/runtime parity, and `git diff --check` pass
  for this merged source state.
- The integration repair's focused CPO, liveops, customerauth, halops, route/
  OpenAPI, and PowerShell documentation checks pass. `go test -p 1 ./...`,
  `go vet -p 1 ./...`, and `git diff --check` pass; serial Go verification
  avoids an environment paging-file limit observed with parallel test process
  startup. `TEST_DATABASE_URL` is unset, so disposable migration and lifecycle
  verification remains pending.
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
