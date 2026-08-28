# WI-20260826-cpo-access-mail-support-completeness

Status: In Progress
Owner: Codex
Collaborators: None
Started: 2026-08-26
Last updated: 2026-08-28

Development-plan reference: `docs/DEVELOPMENT_PLAN.md`
Detailed-plan reference: `docs/plans/cpo-access-mail-support-completeness.md`
Issue/PR reference: None

## Outcome

Make CPO staff authority capability-based and fresh at every decision, replace
generic mail rendering with semantic typed templates and safe frontend action
links, deliver subscription lifecycle notifications through the durable outbox,
and make CPO-to-platform support a bounded, auditable workflow.

## Scope

- `src/auth/`, `src/cpopermissions/`, `src/cpo/`, `src/integrations/`, and CPO
  route wiring.
- `src/mail/`, configuration, and every mail producer affected by templates.
- `src/subscriptions/` lifecycle notification delivery.
- `src/support/`, its models/migrations, routes, tests, OpenAPI, and docs.

## Non-goals

- HAL, payment-provider, deployment, live-data mutation, and direct SMTP.
- Tenant-configurable permission definitions or a new role/entitlement engine.

## Claimed surfaces

- CPO permission registry, evaluator, route middleware, membership APIs.
- Durable mail outbox templates and frontend-link configuration.
- Subscription lifecycle worker notification path.
- Support-ticket lifecycle and HTTP contract.
- CMS documentation and OpenAPI.

## Dependencies and blockers

- None. PostgreSQL lifecycle verification requires a separately selected
  disposable `TEST_DATABASE_URL`.

## Contract impact

- CPO routes become individually capability-authorized.
- Canonical CPO access discovery and effective-permission responses are added;
  the older catalog route remains a documented compatibility alias.
- Support list/detail/history and mutation idempotency contracts are expanded.

## Data and migration impact

- Additive migration only for support lifecycle/history and idempotency state.

## Current state

- The core support workflow is implemented but remains uncommitted: migration
  000057 replaces `PENDING`, adds immutable lifecycle events, and enforces
  locked status/reply mutations. Queue list is cursor-paginated and summary
  only; detail returns messages plus history; replies are durably idempotent.
- Support mail/notification delivery is intentionally not implemented yet. It
  must consume committed support facts without coupling state transitions to
  SMTP.

## Verification

- Focused permission, mail, subscription, support, OpenAPI, and migration tests.
- `go test ./...`, `go vet ./...`, `go build ./...`, docs verification where
  PowerShell is available, and `git diff --check`.

## Handoff

- Preserve the current uncommitted worktree. The next support-only slice is
  notification/mail delivery; do not redesign the finished core status, list,
  detail, lock, or idempotency behavior.

## Completion

- Support core implemented. The whole work item remains in progress because
  support notification/mail delivery and final reconciliation remain.
