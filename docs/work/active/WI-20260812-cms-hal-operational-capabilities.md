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

- HAL operations/reconciliation, shared fact ingress, CMS-projection live reads, inventory-owned pending mappings, CPO/App/Platform operational REST and SSE, durable operational-event publication, migration, OpenAPI, and integration docs.

## Non-goals

- HAL, legacy CMS, OCPP protocol, tariff/GST redesign, Superadmin charger control, physical-charger acceptance, or deployment.

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

- Adds migration 33 for durable scoped operational notifications. It is not applied by this work item.

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

## Verification

- Focused User App and live-projection package tests pass after the batch
  overlay refactor. Earlier focused capability/SSE/OpenAPI route and vet
  evidence remains recorded above; current full-suite and disposable
  lifecycle evidence must be rerun before this work item can close.
- PostgreSQL migration execution, fact/mapping/SSE lifecycle behavior, reconciliation recovery, and dual-service E2E remain pending.

## Handoff

- Do not mark complete until lifecycle/mapping/fact/SSE behavior and topology acceptance are proven.

## Completion

Not complete.
