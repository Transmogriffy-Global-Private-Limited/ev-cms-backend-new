# WI-20260820-cms-hal-fact-delivery-closure

Status: Completed
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-20
Last updated: 2026-08-20

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — Charging lifecycle and HAL integration
Detailed-plan reference: `docs/integrations/ocpp-hal-boundary.md`
Issue/PR reference: None

## Outcome

Close the CMS side of durable signup replacement and the authenticated
platform-triggered HAL exact-fact requeue path.

## Scope

- Replace an existing current signup challenge atomically before a new Start.
- Add the CMS HAL requeue wire client, platform authorization, audit/event
  evidence, route, contract tests, and documentation.

## Non-goals

- Database deployment, migration application, physical charger operation, or
  changing CMS/HAL state ownership.

## Claimed surfaces

- `src/customerauth`, `src/halclient`, `src/halops`, `src/platformops`, routes,
  OpenAPI/contracts, tests, and project memory.

## Dependencies and blockers

- Coordinates with `WI-20260812-cms-hal-operational-capabilities`.
- Disposable PostgreSQL is needed for concurrency/lifecycle confirmation.

## Contract impact

- Adds a protected platform operator command that invokes only HAL's existing
  authenticated exact-fact requeue endpoint.

## Data and migration impact

- Relies on the already-applied migration 46 current-signup uniqueness index;
  no CMS migration is added.

## Current state

Implemented in source. The CMS route remains inactive in practice until HAL is
configured; no deployment was requested.

## Verification

Passed: documentation verification, OpenAPI/runtime parity, focused
customer-auth/HAL-client/platform tests, serial `go test -p 1 ./...`, serial
`go vet -p 1 ./...`, and `git diff --check`. PostgreSQL-dependent concurrency
and lifecycle tests skip without `TEST_DATABASE_URL`.

## Handoff

Do not publish until the paired HAL claim-token and configuration changes are
verified and CMS/HAL contracts agree.

## Completion

Complete; archive after publication.
