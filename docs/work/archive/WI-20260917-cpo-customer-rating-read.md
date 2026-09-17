# WI-20260917-cpo-customer-rating-read

Status: Complete
Owner: Codex
Started: 2026-09-17
Completed: 2026-09-17

## Outcome

Exposed persisted customer ratings to authorized CPO staff through a
tenant-scoped read-only list endpoint and rehosted the new main revision.

## Contract and invariants

- `GET /api/v1/cpo/customer-ratings`, authorized by active CPO membership,
  matching `X-CPO-App-ID`, and `customers.read`.
- Tenant scope derives from the authenticated principal. Filters and cursors
  cannot select another tenant.
- Newest-first bounded keyset pagination; rating filters are inclusive 1–5.
- Customer email is personal data; user-written review text is untrusted and
  must be rendered as text, not HTML.
- No rating create/update/delete route is included.

## Changes

- Implemented the CPO route, typed query/response, service, and CPO-scoped
  repository query with customer/charger/hub/session preloads.
- Added unit tests for query parsing, service validation, tenant-principal
  propagation, projection and page cursor behavior.
- Added PostgreSQL integration coverage for tenant isolation, preloads, rating
  filtering and continuation pages. It was not run because `TEST_DATABASE_URL`
  is unset; no hosted database was used for tests.
- Synchronized OpenAPI, administrative API contract, CPO frontend/backend
  handoffs, schema, plan, project state, changelog and hosting record.

## Deployment and verification

- Deployed source revision `64504d5`; no migration or configuration change was
  needed (migration 71 remains current). The active binary SHA-256 is
  `c072ae5bbf8e5d7d108af59960a776f9ce45a99ce9c96987747cb797293af850`.
- Previous binary retained at
  `/root/evcmsnew-backups/pre-customer-rating-read-64504d5-20260917T095439Z/evcmsnew`
  (SHA-256
  `5ebd8ac1853750bbfbe6b3dabbd880aadb05cb24208e002447d927d7d15c6cb0`).
- Post-rehost PID 5244 is active with zero restarts and matching process/install
  hashes. Local/public liveness, readiness and docs return 200; OpenAPI is 247
  operations; seven required workers are healthy; Caddy validates; no new
  process errors. The unauthenticated endpoint returns 401.
- Mail has 509 SENT rows and zero pending/processing; the latest SENT timestamp
  predates rehost, so no delivery is attributed to the release.
- `go test -p 1 ./...`, vet, production build and route/OpenAPI parity pass.
  The explicit PostgreSQL integration test skipped because `TEST_DATABASE_URL`
  is unset. `pwsh` is unavailable, so `scripts/verify-docs.ps1` was not run.
  Physical OCPP and SMTP acceptance remain unverified.
