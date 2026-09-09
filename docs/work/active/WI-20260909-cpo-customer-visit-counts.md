# WI-20260909-cpo-customer-visit-counts

Status: Implemented (deployed; PostgreSQL lifecycle verification pending)
Owner: Codex
Collaborators: Anubhab Dey (CPO/backend boundary owner)
Started: 2026-09-09
Last updated: 2026-09-09 (rehost and public verification complete)

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — CPO customer directory
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Expose a CPO-authorized, tenant-scoped read of customer charging-session visit
counts, total usage, and latest session time.

## Scope

- `GET /api/v1/cpo/customers/visit-counts` with bearer/app-ID authority,
  bounded customer search, and keyset pagination.
- Aggregates only `COMPLETED` and `RECONCILIATION_REQUIRED` sessions; the read
  does not call HAL or mutate customer, session, wallet, or charger state.
- OpenAPI and human administrative contract documentation.

## Non-goals

- Customer mutation, charging control, settlement changes, HAL changes, new
  persistence, or physical-charger acceptance.

## Claimed surfaces

- `src/cpo/router.go`, `src/cpo/service.go`, `src/cpo/schemas.go`
- OpenAPI and `docs/contracts/api/administrative-http-api.md`
- CPO project-state, deployment, and verification documentation

## Dependencies and blockers

- Reuses the existing CPO customer read capability and tenant list cursor.
- PostgreSQL lifecycle tests remain unavailable without an explicitly selected
  disposable `TEST_DATABASE_URL`.

## Contract impact

Adds one CPO staff GET route with `q`, `limit`, `before`, and `before_id`
parameters and a safe customer aggregate response.

## Data and migration impact

None. Existing customer and charging-session tables remain authoritative.

## Current state

Implemented and deployed in the runtime built from source revision `b21020e`
plus contract alignment. Live and source OpenAPI contain 242 operations.

## Verification

Focused CPO/routes tests, full `go test -p 1 ./...`, `go vet -p 1 ./...`,
production build, post-rehost process identity, loopback/public health and
readiness, OpenAPI/Swagger, worker, Caddy, and startup-log checks pass.
PostgreSQL lifecycle coverage remains skipped because `TEST_DATABASE_URL` is
unset; `pwsh` is unavailable for `scripts/verify-docs.ps1`.

## Handoff

Preserve the authenticated CPO scope, aggregate status allowlist, and bounded
cursor semantics. Do not add a second customer aggregate source of truth.

## Completion

Deployment and public verification complete; dynamic PostgreSQL lifecycle
verification remains pending.
