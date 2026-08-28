# WI-20260826-cpo-access-mail-support-completeness

Status: Verified (uncommitted)
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

- Support core is committed on `main` at `1ebcdf6`: migration 000057 replaces
  `PENDING`, adds immutable lifecycle events, and enforces locked
  status/reply mutations. Queue list is cursor-paginated and summary only;
  detail returns messages plus history; replies are durably idempotent.
- The current uncommitted slice adds transactionally coherent encrypted mail
  intents for ticket creation, platform replies, and platform
  resolved/closed/reopened transitions. CPO activity emits durable platform
  events instead of guessing a platform support email recipient. The mail
  worker remains the only SMTP delivery owner.

## Verification

- Focused permission, mail, subscription, support, OpenAPI, and migration tests.
- `go test ./...`, `go vet ./...`, `go build ./...`, docs verification where
  PowerShell is available, and `git diff --check`.
- Support notification completion: `go test ./src/support ./src/mail -count=1`,
  `./scripts/verify-docs.ps1`, `go test ./...`, `go vet ./...`, `go build ./...`,
  and `git diff --check` passed on 2026-08-28. PostgreSQL-backed mutation and
  concurrency coverage remains gated by an unset disposable `TEST_DATABASE_URL`.

## Handoff

- All scoped implementation and source verification is complete. A future
  publication step may stage this explicit worktree, commit it, and push only
  with fresh human authorization; it must not deploy, apply migrations, or
  contact SMTP as part of publication.

## Completion

- Complete. The work item is archived because its implementation, contracts,
  documentation, and source verification are reconciled. No deployment,
  migration execution, SMTP contact, commit, or push occurred in this slice.
