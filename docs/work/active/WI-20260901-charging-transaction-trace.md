# WI-20260901-charging-transaction-trace

Status: Verified
Owner: Codex
Collaborators: None
Started: 2026-09-01
Last updated: 2026-09-01 (implementation and local verification complete)

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` (charging lifecycle and CPO operations)
Detailed-plan reference: User-approved first-class charging transaction trace / waterfall specification
Issue/PR reference: None

## Outcome

Provide a CPO-readable, connector-aware, immutable diagnostic trace that joins
CMS application/commercial evidence with HAL OCPP and charger-state evidence
without becoming a second source of truth for charging, billing, or connector
state.

## Scope

- CMS trace persistence, charging start/stop/fact evidence, authorized CPO
  read APIs, HAL trace merge, contracts, frontend handoff, and tests.
- Cross-repository coordination with the matching HAL work item.

## Non-goals

- Changing charging, settlement, connector, or OCPP authority.
- Applying migrations, deployment, or direct client-to-HAL access.

## Claimed surfaces

- `src/customerauth/`, `src/halclient/`, CPO authorization/routes, models and
  migrations, OpenAPI, CPO handoff, charging tests.

## Dependencies and blockers

- Depends on HAL's private authenticated trace-read contract and durable trace
  evidence. No external blocker is currently known.

## Contract impact

- Adds CPO trace queries and exposes opaque trace identity where a session or
  start command needs diagnostic correlation. CMS remains the only frontend
  entry point and returns partial HAL state when the private HAL read fails.

## Data and migration impact

- Additive CMS migration only; do not apply it during this work.

## Current state

- Baseline: CMS `anubhab-work` at `184ea10de0f1926cd209bb2b013b598a8804241f`.
- CMS migration 000059, trace model, pre-command opaque ID propagation,
  sanitized APP/CMS evidence, CPO capability/routes, private HAL merge, bounded
  cursor validation, and CPO waterfall handoff are implemented in the current
  publication-ready source worktree.
- Trace evidence is explicitly non-authoritative. CMS trace retention is
  bounded by `PLATFORM_CHARGING_TRACE_RETENTION`; it does not mutate session,
  command, fact, wallet, or connector authority.
- CMS focused/full local verification and documentation/OpenAPI parity pass.
  Final paired HAL reconciliation and final-status checks remain.

## Verification

- Focused CMS trace, charging, route, and OpenAPI tests; full Go checks and
  documentation verification. PostgreSQL lifecycle coverage only runs when an
  explicit disposable `TEST_DATABASE_URL` is already configured.

## Handoff

- Start from this item and the paired HAL work item. Preserve the existing
  source-of-truth and fact-outbox boundaries.

## Completion

- Implementation and local verification are complete. Publication is
  authorized; migration application, deployment, and database mutation remain
  out of scope.
