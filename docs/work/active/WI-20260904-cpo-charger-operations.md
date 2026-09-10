# WI-20260904-cpo-charger-operations

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-09-04
Last updated: 2026-09-10 (source-only TriggerMessage accepted/window atomicity and final-closure correction; paired deployment and hardware validation pending)

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
- Per-operation trace identity, safe OCPP CALL/CALLRESULT/CALLERROR evidence,
  lazy CPO operation-evidence projection, and audited GET_CONFIGURATION.
- Diagnostic-only 60-second accepted TriggerMessage follow-on observation;
  it is temporal evidence, not command, charger-effect, or charging truth.

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

Implemented and deployed CMS: stable operation trace identities,
operation-root linking, strict action-specific protocol evidence sanitation,
the CPO-only grouped exchange projection, and audited GET_CONFIGURATION with
requested-key-only history and an optional transient safe response. CMS
migration `000063` is applied; counterpart HAL migration `021` remains
unapplied.

Source-only, not deployed: paired HAL/CMS follow-on trace evidence for an
allowlisted TriggerMessage durably accepted through HAL's persisted
`CALLRESULT` trace. HAL migration `022` (unapplied) atomically persists the
accepted trace, outbox record, and indexed durable window before later
operation completion. It permits only `OPEN -> OBSERVED` or `OPEN -> CLOSED`,
and the existing trace worker closes expired `OPEN` windows. CMS derives
acceptance from delivered trace evidence,
returns `NOT_OBSERVED` only from a matching delivered closure, keeps missing
delivery `PENDING` past 60 seconds. Its positive-first scan is defensive only,
not a correction of contradictory HAL closure. Requested action, charger identity, scoped connector for
MeterValues/StatusNotification, strict temporal bounds, overlap, and the
non-causal/no-command-session-connector-financial-truth invariant remain.

Deployed CMS: migration `000064` owns the CPO/customer-scoped vehicle table and
read route; migration `000065` adds the already supported `GET_CONFIGURATION`
kind to the bounded CMS operation constraint. Its rollback refuses to
invalidate existing audited reads, and the history OpenAPI kind filter now
matches the durable catalog. The embedded OpenAPI schema correction is active;
source and live contracts each expose 236 operations. Runtime revision
`219c5b1` plus that correction is active, with its previous binary and the
pre-`000065` database dump retained in the hosting guide and project state.

## Verification

History/parser/projection and evidence-redaction tests, CPO capability-route
coverage, route/OpenAPI parity, full `go test -p 1 ./...`, `go vet -p 1 ./...`,
production build, migration/schema checks, and CMS post-rehost
service/contract verification pass. Migrations `000064` and `000065` were
applied after a retained mode-0600 custom-format dump. PostgreSQL-gated
history/protocol integration coverage remains skipped because
`TEST_DATABASE_URL` is absent; no paired charger was selected.

Focused source checks for the TriggerMessage follow-on classifier, ingress
redaction, OpenAPI contract, HAL observation/store/worker handling, full Go
tests, vet, builds, diff checks, and CMS documentation verification pass.
PostgreSQL window/index and paired runtime validation remain pending because
`TEST_DATABASE_URL` and a test charger are unavailable; neither service has
been rehosted for this source-only slice.

## Handoff

Implement the new ledger and exact reconciliation alongside existing charging
commands; do not route generic operations through Start/Stop records.

## Completion

The original CMS execution and this CMS history slice are deployed and
complete. Keep this item active for paired HAL runtime, dual-service, and
physical OCPP acceptance.
