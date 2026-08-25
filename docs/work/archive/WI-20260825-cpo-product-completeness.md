# WI-20260825-cpo-product-completeness

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (shared platform/CPO infrastructure owner)
Started: 2026-08-25
Last updated: 2026-08-25

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — administrative platform completion
Detailed-plan reference: User-approved platform/CPO product-completeness brief
Issue/PR reference: None

## Outcome

Complete the approved CMS-only platform/CPO administration capabilities while
retaining the established mail outbox, membership model, manual subscription
commercial model, charging/HAL boundaries, and tenant isolation.

## Scope

- CPO customer usage and transaction/session projections.
- Permission-based CPO authority, staff lifecycle, mail templates,
  announcements, platform/CPO support, and subscription expiry lifecycle.

## Non-goals

- HAL/OCPP changes, payment automation, deployment, live database mutation,
  or a replacement for existing mail/announcement/worker infrastructure.

## Claimed surfaces

- `src/cpo`, `src/auth`, `src/mail`, `src/superadmin`, `src/subscriptions`,
  `src/platformops`, models/migrations, routes, OpenAPI, and related docs.

## Dependencies and blockers

- Shares broad infrastructure ownership with WI-20260807. This item preserves
  its stated models and records only new contract/invariant work here.
- Disposable PostgreSQL is required for migration and concurrency execution.

## Contract impact

- CPO customer/session/transaction projections become complete and
  human-readable. Later slices add only documented, tenant-scoped APIs.

## Data and migration impact

- New additive migrations are expected for permission overrides, multi-CPO
  announcement targets, support threads, and subscription lifecycle evidence.

## Current state

- Customer total usage, complete charging/session transaction projections,
  CPO staff membership lifecycle, source-controlled permission defaults and
  override persistence, multi-CPO announcement target snapshots, and
  fail-closed multipart mail delivery are implemented in source.
- Additive migrations 49 through 52 are un-applied. The source now includes
  the support thread and subscription lifecycle slices; disposable PostgreSQL
  verification remains required before any database rollout.

## Verification

- Focused auth/CPO/SuperAdmin/mail tests, documentation validation, OpenAPI
  route parity, `go test ./...`, and `go vet ./...` pass.
- Migration/database lifecycle tests remain pending a disposable
  `TEST_DATABASE_URL`.

## Handoff

- Do not replace the durable mail outbox, global User + CPOMembership authority,
  manual subscription commercial model, or HAL-owned transaction truth.

## Completion

Completed in source; repository-wide verification is passing and the requested
commit/publication sequence is in progress.
