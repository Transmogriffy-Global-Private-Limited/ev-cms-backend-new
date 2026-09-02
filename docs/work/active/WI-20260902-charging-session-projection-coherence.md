# WI-20260902-charging-session-projection-coherence

Status: Verified
Owner: Codex
Collaborators: Anubhab Dey (product and CMS/HAL boundary owner)
Started: 2026-09-02
Last updated: 2026-09-02 (customer charger fallback correction deployed and verified)

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — charging lifecycle and CPO operations
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Make customer and CPO active charging-session projections coherent without
changing CMS/HAL ownership, live-event authority, or financial calculation.

## Scope

- Restore a missing customer session charger relation only from the same-CPO
  persisted connector-to-charger relationship.
- Align CPO canonical live-session static context with normal session reads.
- Preserve full-replacement REST/SSE delivery, tenant boundaries, cursor,
  capability checks, frozen tariff/tax display, and current financial
  projection calculation.

## Static-field parity matrix

| Canonical normal-session static context | Canonical live-session form | Notes |
| --- | --- | --- |
| CMS session identity | `session_id` | Existing live identity; no redundant `id` alias. |
| OCPP transaction identity | `ocpp_transaction_id` | Persisted protocol identity, distinct from the CMS session ID. |
| Customer, charger, connector context | `customer`, `charger`, `connector` | Nested canonical views match normal-session semantics; existing flat display fields remain compatibility fields. |
| Session start | `started_at` | Existing live semantic equivalent of normal `start_time`. |
| Initial observed SoC | `initial_soc_percent` | First durable actual observation; latest SoC remains telemetry. |
| Frozen commercial/limit metadata | `price_per_unit`, `unit`, `start_criteria`, `requested_limit_value`, `sgst_percent`, `cgst_percent`, `igst_percent` | Reuse normal-session mapping; do not alter calculation. |
| CMS materialization timestamp | `created_at` | Existing durable session timestamp. |
| Completion-only fields | Omitted while live | Do not invent end time, final SoC, final totals, or stop reason. |
| Live-only state | Existing telemetry/financial fields | Duration, meter/SoC freshness, energy, and projected amount remain live-only. |

## Non-goals

- Pricing/tariff redesign, database migration, HAL changes, database mutation,
  or client-side state reconstruction.

## Claimed surfaces

- `src/customerauth/`, `src/cpo/`, focused tests, OpenAPI, CPO/User App
  frontend handoffs, and project memory.

## Dependencies and blockers

- Customer session materialization persists direct charger identity from the
  start intent. The connector relationship is a same-CPO historical fallback.
- PostgreSQL lifecycle verification remains conditional on an already-selected
  disposable `TEST_DATABASE_URL`.

## Contract impact

- CPO live-session REST snapshots and SSE replacement frames gain canonical
  nested active-session static context while retaining current flat display
  fields for compatibility.

## Data and migration impact

None. Reads use existing persisted session, connector, charger, tariff,
snapshot, and start-intent relationships.

## Current state

The prior published customer fallback required a fully loaded nested
`Connector.Charger` association and did not repair the reported rows. The
source-verified correction uses the same bounded CPO-scoped batch lookup
pattern as the CPO session repository, preferring the persisted connector
charger key and then the materialized session key. REST snapshots and both SSE
frame types continue to use the existing financial wrapper around the same
projection.

## Verification

Passed: focused customer-auth/CPO/liveops/operational-realtime tests, full
`go test ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`.
PostgreSQL-gated coverage requires a disposable `TEST_DATABASE_URL`.

## Handoff

Do not manufacture charger values in serializers. Trace rows remain diagnostic
evidence only; HAL remains the owner of OCPP and meter truth.

## Completion

Source, deployment, and public verification are complete. No migration or
database action was required.
