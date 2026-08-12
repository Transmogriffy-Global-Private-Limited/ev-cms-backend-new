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
- Durable operational events are produced with accepted newer fact projections and consumed through scoped REST cursor replay/SSE. Streams revalidate bearer sessions at heartbeat.

## Verification

- Focused package tests, operational SSE-frame/cursor tests, documentation verification, OpenAPI/runtime route parity, and `go vet -p 1 ./...` passed after refactoring. The combined broad-test/vet runner timed out before returning a full-suite result; the separate vet pass completed.
- PostgreSQL migration execution, fact/mapping/SSE lifecycle behavior, reconciliation recovery, and dual-service E2E remain pending.

## Handoff

- Do not mark complete until lifecycle/mapping/fact/SSE behavior and topology acceptance are proven.

## Completion

Not complete.
