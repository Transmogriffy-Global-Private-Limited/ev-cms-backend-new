# WI-20260831-userapp-realtime-projections

Status: Verified (awaiting review/publication)
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-31
Last updated: 2026-08-31

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — Phase 5/6 customer charging operations
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Replace the User App's new live charging and selected-charger realtime contracts
with full, customer-safe CMS projections over authenticated SSE, while retaining
the existing retained operational-event feed for compatibility.

## Scope

- `src/customerauth/`: customer-scoped live-session and charger-availability
  projection services, authenticated SSE handlers, visibility revalidation, and
  transactional wake-up publication for CMS-owned stop-state changes.
- `src/operationalrealtime/`: correctly scoped committed-event watermarks and
  wake-up queries for projection streams.
- `src/cpo/`: correct the existing CPO live-session stream watermark-before-
  snapshot ordering without changing its public payload or authorization.
- User App/CPO realtime OpenAPI, frontend contract documentation, tests, and
  project memory.

## Non-goals

- HAL, cpconsole, frontend, deployment, migration execution, or direct
  PostgreSQL data changes.
- Removing the legacy generic operational-event endpoints.

## Claimed surfaces

- Customer charging/network projections and routes; operational event queries;
  CPO live-session SSE ordering; OpenAPI/realtime/User App docs; focused tests.

## Dependencies and blockers

- Uses committed CMS projections and existing `liveops` batch readers; no
  synchronous HAL reads.
- PostgreSQL-backed lifecycle tests remain gated on an explicitly supplied
  disposable `TEST_DATABASE_URL` and will not be configured by this work.

## Contract impact

- Adds User App full-state SSE routes for current live sessions and one
  customer-visible charger's availability. Existing generic event routes remain
  retained invalidation/cursor compatibility routes.

## Data and migration impact

- None. Existing charging, runtime, connector, and operational-event records
  remain authoritative.

## Current state

- The User App now has complete replacement SSE projections for current
  customer live sessions and selected customer-visible charger availability,
  with matching JSON recovery for live sessions. The generic retained event
  routes remain compatible but are not the new frontend state contract.
- Customer stop-state mutations publish their committed wake-up in the same
  transaction. Both User App streams reproject on events and on authorized
  heartbeat cadence; the CPO live-session stream now records its watermark
  before it reads its initial projection.

## Verification

- Passed: `gofmt` on changed Go files.
- Passed: `go test -count=1 ./src/customerauth`, `./src/liveops`,
  `./src/operationalrealtime`, `./src/cpo`, and `./src/routes`.
- Passed: `go test ./...`, `go vet ./...`, `go build ./...`,
  `go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1`,
  `./scripts/verify-docs.ps1`, and `git diff --check`.
- Deferred by task/environment: PostgreSQL-gated lifecycle and concurrency
  cases, because `TEST_DATABASE_URL` was not configured or provisioned.

## Handoff

- Preserve the CMS/HAL boundary: operational events wake projections, while
  browser-visible state is always a complete CMS projection. Reconnect begins
  with current state, not historical event reconstruction.

## Completion

- Source implementation, contracts, documentation, and local verification are
  complete. No migration, deployment, restart, direct database mutation,
  commit, merge, or push was performed. Retain this work item in `active/`
  until review/publication changes its coordination state.
