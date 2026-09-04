# WI-20260904-cpo-charger-operations

Status: Implemented
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-09-04
Last updated: 2026-09-04 (CMS history/audit listing deployed; paired HAL and hardware validation pending)

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — Charging lifecycle and HAL integration
Detailed-plan reference: `docs/integrations/cpo-hal-operational-capability-manual.md`
Issue/PR reference: None

## Outcome

Provide CPO-authorized, typed, durable charger-control operations through the
existing CMS `halops -> halclient -> HAL v1 -> OCPP` boundary.

## Scope

- Reset, UnlockConnector, ChangeAvailability, ClearCache, GetConfiguration,
  ChangeConfiguration, and allowlisted TriggerMessage.
- Dedicated CMS and HAL operation ledgers, exact-ID reconciliation, caller
  idempotency, scoped CPO audit/recovery reads, operational invalidations,
  contracts, tests, and forward-only migrations.
- CMS-owned paginated, filtered, safe CPO history listing without HAL calls or
  list-side reconciliation.

## Non-goals

- Direct browser-to-HAL access, generic OCPP passthrough, CPO raw remote
  start/stop, firmware, diagnostics, customer chargeability changes, or
  old-HAL work.

## Claimed surfaces

- CMS `src/cpo`, `src/halops`, `src/halclient`, models, migrations, routes,
  operational events, OpenAPI, integration docs, and project memory.
- Counterpart new-HAL v1 HTTP/store/OCPP operation contract, migration,
  OpenAPI/docs, and work record.

## Dependencies and blockers

- Existing CPO operational capability work item owns adjacent read/realtime
  infrastructure; this slice reuses it without changing charging Start/Stop.
- PostgreSQL lifecycle and real dual-service/charger validation require an
  explicitly selected disposable `TEST_DATABASE_URL`, which is absent.

## Contract impact

Adds typed CPO charger-operation routes and an authenticated HAL v1 operation
contract. CMS operation state, HAL acceptance, OCPP acknowledgement, and later
observed charger effects remain separately represented. The CMS history route
lists every CMS-recorded attempt, including failed or HAL-absent attempts.

## Data and migration impact

Adds forward-only migrations for dedicated operation ledgers. CMS migration
`000061` was applied during the authorized development rehost after a validated
mode-0600 database dump; the paired HAL migration remains a separate runtime
dependency.

## Current state

Implemented and deployed CMS source: dedicated CMS operation records and
migration; scoped
CPO routes, idempotency/digest, server correlation, committed events, typed
HAL calls, exact-ID recovery, and CMS OpenAPI/human contract. The counterpart
new-HAL source has its own operation ledger/migration and typed OCPP dispatch.

Implemented and deployed CMS-only history adds deterministic tenant-rooted
listing, enrichment, bounded real-semantic filters, configuration-value
redaction, and one targeted cursor index. It does not alter the exact-recovery
path or call HAL.

## Verification

History parser/projection tests, CPO capability-route coverage, route/OpenAPI
parity, full `go test -p 1 ./...`, `go vet -p 1 ./...`, production build,
migration/index checks, and CMS post-rehost service/contract verification pass.
PostgreSQL-gated history integration coverage remains skipped because
`TEST_DATABASE_URL` is absent; `pwsh` is unavailable for documentation
verification.

## Handoff

Implement the new ledger and exact reconciliation alongside existing charging
commands; do not route generic operations through Start/Stop records.

## Completion

The original CMS execution and this CMS history slice are deployed and
complete. Keep this item active for paired HAL runtime, dual-service, and
physical OCPP acceptance.
