# WI-20261124: Durable subscription lifecycle work recovery

Status: Source candidate, awaiting local Go verification and disposable PostgreSQL integration acceptance; not deployed.
Baseline: `anubhab-work` `fb4e7d2f80b93c242a108371f84d903a11bd56ee`.

## Purpose

Eliminate permanent deferral of failed/interrupted lifecycle work after the
bounded scanner cursor moves forward into a continually growing eligible set.
Preserve tenant, period-event, history, outbox and customer Stop contracts.

## Claimed code and contract surfaces

- `src/subscriptions/worker.go` and new `lifecycle_queue.go`
- New migration `000077_subscription_lifecycle_pending_work` (up/down)
- Deterministic backoff unit tests and PostgreSQL-gated recovery/fencing tests
- `docs/contracts/api/manual-subscriptions.md`
- `docs/SUPERADMIN_FRONTEND_HANDOFF.md` and `scripts/verify-docs.ps1`
- Current source candidate record in `docs/PROJECT_STATE.md` and `docs/AI_CHANGELOG.md`

## Contract and safety

- Migration 000077 initially queues eligible subscriptions that may already lie
  behind the old cursor, preserving previously uncompleted work on upgrade.
- Commit discovered jobs to `subscription_lifecycle_pending_work` *within the same
  transaction* as cursor advancement. Crash/restart retains unacknowledged work.
- An expiring lease makes abandoned jobs eligible again. Failed jobs use
  capped exponential backoff and do not block fresh scan progress.
- Every claim uses a distinct token; stale acknowledgements/defer operations
  cannot erase or reschedule work now owned by a newer claimant.
- Subscription row lock, per-period unique lifecycle event, and system history
  idempotency remain the final exactly-once effect guards.
- Pending queue includes no secrets or customer details. Server event and API
  contracts are unchanged. 000077 down refuses to discard pending work.
- CPO cancellation and RBAC policy gates are explicitly NOT changed.

## Evidence and gates

- Review source hashes/anchors; local Go compilation, tests, vet, build,
  docs verifier and diff check must pass before treating source as verified.
- Existing `TEST_DATABASE_URL`-guarded integration tests remain skipped without
  an explicitly approved disposable DB. No improvised Windows PG runner.
- PostgreSQL migration execution and physical HAL Stop remain unverified and
  mandatory before deployment. No commit/push/deploy without human approval.
- If a new failing record remains pending indefinitely, inspect queue retry
  deadline/attempts and errors rather than clearing it or moving the cursor.

## Close when

Code is applied and reviewed on actual checkout; all local checks pass;
PostgreSQL migration/recovery concurrency tests and downstream acceptance are
completed with documented evidence. Move item to archive only then.
