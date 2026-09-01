# WI-20260901-cpo-session-commercial-projection

Status: Verified
Owner: Codex
Collaborators: Abhranil Pal (source branch contribution)
Started: 2026-09-01
Last updated: 2026-09-01 (semantic reconciliation, deployment, and verification complete)

Development-plan reference: CPO reporting and session projections
Detailed-plan reference: chronology override for `abhranil_ev_cms_backend_new`
Issue/PR reference: None

## Outcome

Reconcile the useful CPO charging-session commercial display additions from
`origin/abhranil_ev_cms_backend_new` onto current `main`, preserving current
authorization, trace, realtime, and session behaviour.

## Scope

- Session GET/list display of frozen tariff price/unit, tax rates, and customer
  limit selection.
- Snapshot-first historical semantics with current entities only as legacy
  fallback.
- CPO-scoped repository loading, focused regressions, OpenAPI, and frontend
  contract guidance.

## Non-goals

- Pricing, tariff calculation, tax calculation, charging control, migrations,
  or database mutation.

## Claimed surfaces

- `src/cpo/repository.go`, `src/cpo/schemas.go`, `src/cpo/service.go`
- `src/models/schema.go`, CPO projection tests, OpenAPI, CPO handoff/docs

## Dependencies and blockers

- Current `main` is the chronological base. The source branch diverged at
  `0cd2afe`; its two commits are reviewed as intent, not merged mechanically.

## Contract impact

- Extends CPO historical charging-session responses only. Live-session SSE and
  authoritative commercial state remain unchanged.

## Data and migration impact

- None. Existing `ChargingSession` and `ChargingStartIntent` snapshots are
  authoritative for historical display.

## Verification

- Focused projection tests, full Go verification, OpenAPI/docs verification,
  and `git diff --check`. PostgreSQL tests remain conditional on the existing
  `TEST_DATABASE_URL`.

## Handoff

- Preserve snapshot-first tariff/tax display and separate customer limit type
  from tariff billing dimension.

## Completion

- Semantic reconciliation, deployment, and local/public verification are
  complete. No migration or database mutation was required.
