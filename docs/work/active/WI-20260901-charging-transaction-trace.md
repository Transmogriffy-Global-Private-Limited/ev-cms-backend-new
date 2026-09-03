# WI-20260901-charging-transaction-trace

Status: In Progress
Owner: Codex
Collaborators: None
Started: 2026-09-01
Last updated: 2026-09-03 (root-identity enrichment deployed)

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
  read APIs, isolated HAL trace ingress, contracts, frontend handoff, and tests.
- Cross-repository coordination with the matching HAL work item.

## Non-goals

- Changing charging, settlement, connector, or OCPP authority.
- Direct client-to-HAL access.

## Claimed surfaces

- `src/customerauth/`, `src/halclient/`, CPO authorization/routes, models and
  migrations, OpenAPI, CPO handoff, charging tests.

## Dependencies and blockers

- Depends on HAL's dedicated authenticated trace-event delivery and durable
  trace evidence. No external blocker is currently known.

## Contract impact

- Adds CPO trace queries and exposes opaque trace identity where a session or
  start command needs diagnostic correlation. CMS remains the only frontend
  entry point and serves its durable projection without query-time HAL reads.

## Data and migration impact

- Additive CMS migration 000059 applied after a custom-format database backup;
  no authoritative charging or commercial rows were rewritten.

## Current state

- Baseline: CMS `anubhab-work` at `184ea10de0f1926cd209bb2b013b598a8804241f`.
- CMS migration 000059, trace model, pre-command opaque ID propagation,
  sanitized APP/CMS evidence, CPO capability/routes, private HAL merge, bounded
  cursor validation, and CPO waterfall handoff are implemented and deployed in
  revision `3037c46` (219 OpenAPI operations).
- Trace evidence is explicitly non-authoritative. CMS trace retention is
  bounded by `PLATFORM_CHARGING_TRACE_RETENTION`; it does not mutate session,
  command, fact, wallet, or connector authority.
- CMS focused/full local verification, documentation/OpenAPI parity, migration
  application, and public rehost checks pass for the previously published
  pipeline. The linkage/commercial-evidence slice is deployed in CMS revision
  `a46b50a` with 222 OpenAPI operations. It changes no source-of-truth
  behavior; final paired HAL reconciliation and cross-repository status remain
  separate.
- Runtime evidence for trace `2647bfb4-5c18-46e4-b99d-3e92dfe61dad` showed
  that this published root can retain only `cms_start_intent_id` even though
  command and authoritative session evidence exist. The CMS-only remediation
  now monotonically binds the command at durable creation and the CMS, HAL, and
  OCPP transaction identities at authoritative CMS materialization. It is
  deployed in CMS revision `ab1ab54`; no migration or runtime data mutation was
  required.

## Verification

- Focused CMS trace, charging, route, and OpenAPI tests; full Go checks and
  documentation verification. PostgreSQL lifecycle coverage only runs when an
  explicit disposable `TEST_DATABASE_URL` is already configured.

## Handoff

- Start from this item and the paired HAL work item. Preserve the existing
  source-of-truth and fact-outbox boundaries.

## Completion

- The published pipeline and root-identity remediation are deployed and
  verified. Paired HAL reconciliation remains before the cross-repository
  feature can be marked fully complete.
