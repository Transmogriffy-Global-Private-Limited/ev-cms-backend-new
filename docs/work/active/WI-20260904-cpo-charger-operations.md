# WI-20260904-cpo-charger-operations

Status: Implemented
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-09-04
Last updated: 2026-09-04 (CMS migration and development rehost verified; paired HAL and hardware validation pending)

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

## Non-goals

- Direct browser-to-HAL access, generic OCPP passthrough, CPO raw remote
  start/stop, firmware, diagnostics, customer chargeability changes, runtime
  database mutation, deployment, or old-HAL work.

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
observed charger effects remain separately represented.

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

## Verification

Focused CPO/HAL-client/HAL-operations tests, full `go test -p 1 ./...`,
`go vet -p 1 ./...`, production build, migration/table checks, and CMS
post-rehost service, contract, worker, proxy, and log checks pass. PostgreSQL
lifecycle integration, paired HAL runtime, and physical charger validation
remain skipped unless a disposable URL and mapped charge point are supplied;
`pwsh` is unavailable for the documentation verifier.

## Handoff

Implement the new ledger and exact reconciliation alongside existing charging
commands; do not route generic operations through Start/Stop records.

## Completion

CMS implementation and development deployment are complete. Keep this item
active for paired HAL runtime, dual-service, and physical OCPP acceptance.
