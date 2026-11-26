# WI-20261124: Durable subscription lifecycle work recovery

## 2026-11-26 - Deployed runtime evidence (partial work-item closure only)

- Migration `000077` was applied successfully to the **existing populated VPS2 database** as part of `000075`–`000081`; latest migration ledger is 82/`000081`, source release `2a28228fa04aee365b5a93e7c812419102f84d95`.
- The new `subscription-lifecycle` worker is one of seven required current workers reporting healthy recent heartbeats. Overdue TRIAL count fell by one, EXPIRED rose by one; one expiry event and 7d/3d/1d warning events were present; pending queue count was zero. No matching lifecycle error or SQLSTATE in the bounded startup journal scan.
- Source build/test/vet and migrator compilation passed on VPS2, and prior focused Windows/PG source gates were recorded for the release. **Do not infer that every interrupted-lease, retry, fencing, adverse concurrency or long-duration worker scenario was exercised on the populated runtime.** The original work item remains **active** for those outstanding explicit stress gates; physical HAL acceptance is a different domain.
- The opening source-candidate status and predeployment sections below are historical, superseded by this 2026-11-26 runtime note. See [live release record](../../guides/operations/dev-hosting.md).

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
