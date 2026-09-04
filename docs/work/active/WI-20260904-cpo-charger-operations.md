# WI-20260904-cpo-charger-operations

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-09-04
Last updated: 2026-09-04 (source implementation complete; environment validation pending)

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
  contracts, tests, and source-only migrations.

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

Adds forward-only source migrations for dedicated operation ledgers. They must
not be applied in this task.

## Current state

Implemented source: dedicated CMS operation records and migration; scoped
CPO routes, idempotency/digest, server correlation, committed events, typed
HAL calls, exact-ID recovery, and CMS OpenAPI/human contract. The counterpart
new-HAL source has its own operation ledger/migration and typed OCPP dispatch.

## Verification

CMS documentation verification, focused CPO/HAL-client/HAL-operations tests,
full `go test -p 1 ./...`, `go vet -p 1 ./...`, and diff checks pass.
PostgreSQL integration tests remain skipped unless a disposable URL is supplied.

## Handoff

Implement the new ledger and exact reconciliation alongside existing charging
commands; do not route generic operations through Start/Stop records.

## Completion

In progress.
