# WI-20260820-auth-readiness-invariants

Status: Implemented; source verification complete
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-20
Last updated: 2026-08-20

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — authentication, platform operations, and schema integrity

Detailed-plan reference:

- `docs/plans/customer-authentication.md`
- `docs/plans/customer-signup.md`

Issue/PR reference: None

## Outcome

One current authentication challenge is enforced per logical identity,
password-mutation authorization is serialized, and readiness depends on the
enabled workers expected by the current process incarnation.

## Scope

- Administrative, customer, and signup challenge issuance/replacement
- Administrative and customer password mutation serialization audit
- Worker expected-set, heartbeat, and readiness plumbing
- Additive PostgreSQL constraints and guarded PostgreSQL concurrency tests
- Verified model/schema contract corrections and affected documentation

## Non-goals

- No charging lifecycle, TotalKWh, HAL protocol, deployment, or live-database change
- No authentication rewrite or unverified schema cleanup

## Claimed surfaces

- `src/auth/`, `src/customerauth/`, `src/platformops/`, `src/halops/`, `src/operationalrealtime/`, `src/mail/`, and `main.go`
- additive migrations and guarded PostgreSQL integration tests
- authentication, schema, platform-operations, project-state, plan, and changelog documentation

## Dependencies and blockers

- Disposable PostgreSQL coverage requires an explicitly selected `TEST_DATABASE_URL`.

## Contract impact

- No new public endpoint. `/health/ready` now truthfully rejects missing, stale,
  degraded, or wrong-incarnation enabled required workers.

## Data and migration impact

- Additive migration 46 rejects pre-existing duplicate active logical challenges
  with a diagnostic; it does not delete or arbitrarily invalidate history.
- It refuses to require `gsts.state` while any existing row lacks a meaningful
  state, so business data must be repaired deliberately before applying it.

## Current state

- Administrative and customer login recheck credentials/state under the owner
  lock before creating a challenge. Reset/resend/verification use the same
  owner-before-challenge order; signup uses a CPO/email transaction advisory lock.
- Expected-worker readiness is keyed to this process's generated worker keys;
  HAL reconciliation and retention now report durable worker health.
- Provider-omitted wallet recharge payment methods remain null in Go and SQL.
  Historical `connectors.connector_max_capacity` was audited and retained.

## Verification

- Passed: focused auth/customer-auth/platform-ops tests;
  `go test -p 1 ./...`; `go vet -p 1 ./...`; `scripts/verify-docs.ps1`; and
  focused compile checks for HAL reconciliation and operational retention.
- Present but skipped: migration, concurrent challenge, concurrent password,
  and expected-worker PostgreSQL tests, because `TEST_DATABASE_URL` is unset.
- No migration, deployment, restart, or live-data change was performed.

## Handoff

- Preserve migration `000045` and the completed charging reconciliation/TotalKWh slice.
- Apply migration 46 only after inspecting duplicate-current challenge and null
  GST-state diagnostics against a selected non-production target.

## Completion

- Source implementation and non-database verification are complete. PostgreSQL
  lifecycle evidence remains pending a disposable test database.
