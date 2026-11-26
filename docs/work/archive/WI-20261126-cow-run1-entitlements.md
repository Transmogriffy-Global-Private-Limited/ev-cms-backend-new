# CoW Coding Run 1: default-off module authorization

Status: Source work complete, accepted and archived 2026-11-26. Not a VPS2 deployment.

Authority: `anubhab-work` published Run 1 `f2be703b1d200eb0173c64ba655124496fb2755c`; followed by Run 2A `77d2d85885d2fa4db8559ccbeb7c8049a0d9f974`. Original September blueprint role/DENY decision remains; Go-CMS integration supersedes standalone CMS.

## Source completion evidence (supersedes historical candidate text below)

- Source commit `f2be703b1d200eb0173c64ba655124496fb2755c` was pushed; Go source, contract, vet/build and isolated PG17 entitlement/default-off/authorization/audit/concurrency-version/DOWN checks passed. The GSTIN test-fixture checksum issue was fixed before final acceptance.
- FE handoff complete; canonical repo contract: `docs/contracts/api/cow-module-entitlement.md`.
- Excluded from closure: VPS2 deployment, populated-database upgrades, historical unrelated PostgreSQL regressions and HAL/physical charger validation.

## Original pre-release scope (historical)

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
