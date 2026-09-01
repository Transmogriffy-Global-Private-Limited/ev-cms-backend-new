# WI-20260831-cpo-uac-authority-coherence

Status: Completed
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-31
Last updated: 2026-09-01

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — CPO administration
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Align CPO route, service, session, OpenAPI, and frontend authority behavior
with the source-controlled capability catalogue, preserving tenant isolation and
fresh permission overrides.

## Scope

- Support create/reply capability semantics and no-store protection.
- Integration read/manage route alignment.
- CPO role-change session revocation and authorization infrastructure errors.
- CPO OpenAPI AND security, capability-oriented documentation, and focused
  route/session regressions.

## Non-goals

- Deployment, migration application, credential exposure, role-based bypasses,
  new permission keys, or a general authorization redesign.

## Claimed surfaces

- `src/auth`, `src/cpopermissions`, `src/cpo`, `src/support`,
  `src/integrations`, routes/OpenAPI, CPO frontend documentation, and tests.

## Dependencies and blockers

- PostgreSQL lifecycle tests require an explicitly selected disposable
  `TEST_DATABASE_URL`; none will be created for this work.

## Contract impact

- Support reply requires both `support.read` and `support.reply` because the
  response returns full ticket history. Integration reads require
  `settings.read`; writes require `settings.manage`.

## Data and migration impact

- None expected. Existing session revocation and permission-override models are
  reused transactionally.

## Current state

- Completed the source UAC coherence slice without a role-based bypass: route
  capabilities are the endpoint decision and roles are only default bundles.
- Corrected support, integrations, CPO session revocation, no-store, fresh SSE
  authorization, OpenAPI Bearer-plus-App-ID security, and stale documentation.

## Verification

- PASS: focused `auth`, `cpopermissions`, `middleware`, `cpo`, `support`, and
  `integrations` package tests.
- PASS: OpenAPI/runtime route regression, documentation verification,
  `go test ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`.
- SKIPPED: PostgreSQL-backed lifecycle/concurrency cases because no disposable
  `TEST_DATABASE_URL` is configured; no database was created or modified.

## Handoff

- No active blocker. Future work must preserve capability-as-authority,
  active-membership/app-ID tenant checks, and explicit session-revocation
  reasons; do not reintroduce a role gate below protected routes.

## Completion

- Completed and ready to archive. No migration, deployment, commit, or push was
  performed by this slice.
