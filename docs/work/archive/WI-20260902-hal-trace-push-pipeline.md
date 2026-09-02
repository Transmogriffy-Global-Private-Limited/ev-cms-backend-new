# WI-20260902-hal-trace-push-pipeline

Status: Verified (source only)
Owner: Codex
Collaborators: None
Started: 2026-09-02
Last updated: 2026-09-02

Development-plan reference: `docs/DEVELOPMENT_PLAN.md`
Detailed-plan reference: Task 2 — isolated HAL to CMS diagnostic trace pipeline
Issue/PR reference: None

## Outcome

Replaced query-time CMS-to-HAL charging-trace merging with an isolated durable
HAL diagnostic delivery pipeline and CMS-local CPO static/SSE reads.

## Scope

- CMS trace ingress, trace-root/event persistence, CMS-only CPO reads and SSE.
- Dedicated service authentication, contracts, retention, tests, and documents.
- Coordinated matching HAL delivery outbox/worker work item.

## Non-goals

- Any change to `/v1/hal-facts`, charging authority, billing, deployment, or
  database mutation.

## Claimed surfaces

- `src/cpo/`, trace ingress/realtime modules, models, migrations, config,
  route/OpenAPI/contracts, and trace documentation.

## Dependencies and blockers

- Matching HAL trace-delivery envelope and dedicated bearer are implemented in
  the counterpart source worktree.
- PostgreSQL integration verification remains gated on the pre-existing
  `TEST_DATABASE_URL`; it was unavailable for this work.

## Contract impact

- CPO trace reads are CMS-local. HAL diagnostic events enter only through
  `POST /v1/hal-trace-events` using a distinct service credential.

## Data and migration impact

- Additive CMS migration 000060 creates trace roots plus immutable digest and
  durable ingestion replay fields. It was not applied.

## Current state

- Source was recovered from CMS `7c022761` and HAL `88f10f4e` clean baselines.
- The private HAL trace read and query-time merge are retired from the CPO
  product path. CMS does not call HAL to serve historical trace or SSE replay.

## Verification

- Focused ingress/read/SSE, retained fact-ingress, runtime OpenAPI, docs, full
  Go, vet, build, and diff checks were completed across both repositories.
- PostgreSQL-gated cases were skipped because `TEST_DATABASE_URL` was unset.

## Handoff

- Preserve diagnostic-evidence-only semantics and strict separation from
  authoritative fact delivery. Apply migrations 000060 and HAL 019 only in an
  explicitly authorized coordinated rollout.

## Completion

- Source implementation and applicable local verification complete. No commit,
  push, deployment, database mutation, migration application, or service
  restart was performed.
