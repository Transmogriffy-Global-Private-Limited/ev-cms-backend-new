# WI-20260916-customer-rating-schema-serial-identity

Status: Complete (CMS schema/runtime deployment; no customer rating API)
Owner: Codex
Started: 2026-09-16
Completed: 2026-09-16

## Outcome

Deploy CPO-scoped charger serial-number uniqueness and customer-rating storage
without colliding with the already-applied migration 70 or claiming an
unimplemented customer-facing rating capability.

## Changes

- Migration `000071_customer_ratings` adds the ratings table, rating range and
  per-session uniqueness checks, composite CPO/entity foreign keys, and a
  partial unique index for non-empty charger serial numbers per CPO.
- Corrected GORM relation keys for rating collections and translated the
  serial-index violation to `409 charger_serial_number_conflict`.
- Updated the human/OpenAPI charger error docs and documented rating storage as
  groundwork only. Submission/read routes and their authorization/API contract
  remain unimplemented.
- The incoming branch had mistakenly assigned customer ratings migration 70,
  which was already deployed for charger-operation recovery. This slice
  renumbers ratings to 71 and leaves the applied migration immutable.

## Deployment and verification

- Live preflight found migration 70 current, no rating table, no non-empty
  per-CPO serial duplicates, no pending/processing mail, seven healthy current
  required workers, and `.env` mode `0600` with the same 64 key names as
  `.env.example` (no values exposed).
- Retained the old binary and validated mode-0600 custom dump under
  `/root/evcmsnew-backups/pre-000071-customer-ratings-20260916T110548Z/`.
- Stopped the CMS before applying migration 71, verified the table, constraints
  and indexes, installed/rehosted the new build, and observed zero restarts.
  The rating table is empty; mail remains 508 SENT with zero pending/processing.
- Active binary SHA-256:
  `5ebd8ac1853750bbfbe6b3dabbd880aadb05cb24208e002447d927d7d15c6cb0`.
  Deployed source commit: `38ce906`.
- Focused CPO/routes/model tests, route/OpenAPI parity, full `go test -p 1 ./...`,
  `go vet -p 1 ./...`, production build, and `git diff --check` passed. Runtime
  process/install hashes match; local/public liveness/readiness/docs return
  200; OpenAPI remains 246 operations; all seven required workers are healthy;
  Caddy validates; no new-process error logs were found.
- `TEST_DATABASE_URL` is unset, so dedicated PostgreSQL integration tests were
  not run. `pwsh` is unavailable, so `scripts/verify-docs.ps1` was not run.
  Physical charger/OCPP and SMTP acceptance remain outside this verification.

## Recovery

The replaced binary is retained at
`/root/evcmsnew-backups/pre-000071-customer-ratings-20260916T110548Z/evcmsnew`.
The pre-migration database dump is retained beside it. Migration 71 is
additive; prefer a forward correction over down migration after customer data
exists. The rating table currently contains zero rows.
