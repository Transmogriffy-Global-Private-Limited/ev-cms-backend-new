# WI-20260818-tariff-gst-commercial-correction

Status: Completed
Owner: Anubhab Dey
Collaborators: Codex (implementation)
Started: 2026-08-18
Last updated: 2026-08-18

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — charging commercial lifecycle

Detailed-plan reference:

- `docs/plans/tariff-gst-commercial-correction.md`

Issue/PR reference: None

## Outcome

Correct tariff units, tariff pricing interpretation, Hub-owned GST integrity,
and the downstream charging commercial facts as one compatible CMS-only slice.

## Scope

- Canonical `kwh` energy tariffs while preserving existing per-kWh numeric values
- Shared pricing for admission holds and completed-session settlement
- Immutable new tariff/tax snapshots plus explicit historical readers
- One reusable GST-to-Hub validator at mutation and runtime boundaries
- Hub/GST mutation locking and resulting-relationship validation
- Explicit non-billing policy for unsupported idle fees
- Affected migration, API/OpenAPI, tests, and documentation

## Non-goals

- HAL/OCPP protocol behavior, existing session duration cutoff, liveness,
  realtime, authentication, deployment, or direct database mutation
- Changing tariff precedence, target ownership, schedule semantics, or putting
  GST back on tariffs

## Claimed surfaces

- `db/migrations/000041_*`, tariff/GST migration tests
- `src/constants/`, `src/cpo/`, and `src/customerauth/`
- CPO/customer API contracts, OpenAPI, and commercial documentation

## Dependencies and blockers

- Supersedes the completed migration-40 tariff semantic correction where its
  `watt/hour` basis conflicts with the clarified per-kWh domain contract.
- Overlaps CPO network/pricing work owned by Abhranil Pal; the user explicitly
  owns this corrective commercial subsystem slice.
- PostgreSQL lifecycle verification requires a disposable `TEST_DATABASE_URL`.

## Contract impact

- New energy tariffs and current customer pricing use `units: kwh`; new writes
  reject `watt/hour`.
- Existing snapshots retain explicit legacy readers and are never silently
  reinterpreted.

## Data and migration impact

- A forward migration will replace the persisted `watt/hour` enum value with
  `kwh` without altering stored tariff numeric values. It must not rewrite
  migrations 40 or earlier.

## Current state

- Current `main` prices energy `price_per_unit` as per Wh even though historic
  commercial values represent per kWh. GST assignment/replacement has local
  state checks, while later GST and Hub mutations do not preserve that
  relationship invariant.

## Verification

- Database-free tariff/GST, customer pricing, migration, documentation, and
  OpenAPI/runtime-route checks passed.
- `go test -p 1 ./...`, `go vet ./...`, and `git diff --check` passed.
- Guarded PostgreSQL lifecycle coverage remains pending an explicitly selected
  disposable `TEST_DATABASE_URL`.

## Handoff

- Keep actual HAL start/completion timestamps authoritative for time pricing;
  do not alter the independent duration cutoff.

## Completion

Implemented and documented. The source is not deployed; migration 42 remains
pending normal migration-controlled release.
