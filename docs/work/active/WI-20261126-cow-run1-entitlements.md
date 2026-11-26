# CoW Coding Run 1: default-off module authorization

Status: implementation candidate; user checkout execution not yet shown.

Authority: `anubhab-work` at `dc006722706333aca403c2101eb868c0cdadeac3`. The September 24 blueprint's role-permission and DENY semantics remain normative; later owner-approved project decision supersedes its standalone CMS repository layout. CoW extends this Go EV-CMS and uses existing CPO identity, active membership, current permission overrides and Platform administration.

## Contract and boundaries

- Existing CPOs remain CoW-disabled until a Platform admin explicitly grants the module. A missing entitlement row means disabled, version 0. Ordinary CPO staff cannot grant it.
- `GET /api/v1/platform/cpos/{cpo_id}/modules/cow` reads this state, including version.
- `PUT /api/v1/platform/cpos/{cpo_id}/modules/cow` with `enabled`, `expected_version` and `reason` changes it atomically. Different-current-version requests receive 409; same-state requests at current version are idempotent no-ops.
- Every state change writes append-only `cpo_module_entitlement_events` and existing platform `audit_logs` in one transaction, under CPO row lock.
- `GET /api/v1/cpo/cow/access` requires a signed-in CPO principal with a matching `X-CPO-App-ID` and current active membership. It exposes only the authenticated CPO's CoW enablement and currently effective `cow.*` keys. A suspended CPO or disabled module has zero effective CoW permissions.
- `cow.Authorize` and `cow.RequirePermission` are the reusable fail-closed gates for future CoW operations. Both CPO entitlement and current membership/permissions are checked afresh per request. Explicit DENY wins over role defaults and ALLOW.
- No CoW fleet, driver, booking, payment or HAL action is enabled in this run. Permission keys are the source-controlled future capability vocabulary; a key does not imply its API is implemented.
- Database writes performed *after* a permission check can race a subsequent revocation. Future state-changing CoW services must re-evaluate authorization in their own transactions under compatible locking; do not describe middleware-only checks as retroactive cancellation.
- Rollback of 000080 intentionally refuses destructive loss of grant history; forward-only migration changes are the reviewed production recovery path.

## Verification and release gates

- Read-only guarded source application; backup of modified files; no staging/commit/push.
- Source-only: `go test -p 1 -count=1 ./src/cpopermissions ./src/cow ./src/cpo ./src/routes`, `go vet -p 1 ./...`, `go build -p 1 ./...`, OpenAPI validation and `git diff --check`.
- Isolated PostgreSQL 17: `TEST_DATABASE_URL` scoped to disposable DB, `go test -p 1 -count=1 ./src/cow`, verify default-off, audited enable, DENY, CPO isolation, stale version, no-op and revoke. Inspect migration 000080 in fresh database. Do not use production/test-shared DB for migrations.
- Existing full PG suite's 11 last-recorded failures, historical 000056 provenance, populated upgrade and physical HAL acceptance remain separate open gates. This candidate is not a production deployment claim.
