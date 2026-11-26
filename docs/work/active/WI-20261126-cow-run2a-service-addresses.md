# CoW Coding Run 2A: saved service addresses and service-context snapshots

**Source baseline**: `anubhab-work` at `f2be703b1d200eb0173c64ba655124496fb2755c` (Run 1). This is a guarded candidate, not an applied, tested, committed, or deployed source change.

## Purpose and scope

Canonical September 24 Program 2 (B02.3, B02.4 and prerequisite B02.5 input semantics), adapted to the active Go EV-CMS. Reuse the existing `/api/v1/app/profile` and `/api/v1/app/vehicles` authority. Saved service addresses are new and separate from the CMS charge-point CPO registered business address. A customer must be in an active CPO with CoW explicitly enabled and must have a current, active customer session.

### Implemented in this candidate

- Forward migration `000081` creates CPO/customer composite-FK saved service addresses with a partial unique default index and safe coordinates/version constraints; DOWN refuses destructive rollback.
- Six authenticated customer routes under `/api/v1/app/addresses`: list, create, get, patch, soft-remove and choose default. Every read/write scopes by both CPO and customer. No guessed tenant ID accepted in API.
- First saved address defaults automatically; default changes and removals serialize on the owning customer, clear the previous default, update versions, and never expose deleted master rows. A deleted default is replaced by the oldest still-active saved address, if any. Stale mutations receive `409 address_version_conflict`.
- Mutations audit field names and state changes, not raw address data; any transaction failure rolls back all steps. No-change PATCH/default selection doesn't fabricate an audit entry.
- Internal `BuildCowServiceContext` validates exactly one saved/temporary vehicle and saved/temporary service address, checking ownership and CoW access, and returns copied-value snapshots. It creates no ServiceJob, saved object, or privileged side effects.
- Existing vehicle CRUD shape is unchanged in 2A. No hardware compatibility fields invented where CMS currently lacks verified charging specifications.

### Not implemented / acceptance dependent

- Vehicle charging-compatibility extension (`battery_capacity_wh`, AC/DC support, connectors etc.) belongs to Run 2B after non-breaking model+API design; do not claim that an arbitrary existing vehicle is chargeable.
- ServiceJob/quotes, materialized historical snapshot storage, historical promotion, geocode/route verification, dispatch, pricing and driver actions are future-domain boundaries. Returned snapshots are only immutable data values until a later job transaction persists them.
- No cross-request idempotency key for *create* is claimed; retries after an ambiguous response may create a second saved address. Idempotent semantic no-op is supported for PATCH/default selection; the future service request/job layer requires real idempotency keys.
- No live Go compile or PG17 integration results are yet evidenced for this patch. The owner must apply in an isolated checkout and run the guarded verifier and tests before committing. The original Run 1 commit is untouched.

## Test gates

1. Verify working branch, origin, pinned HEAD and clean workspace, plus Git blob hashes of all modified files. Apply exactly six new files and four intended modifications using the bundled guard; inspect diff.
2. `go test -p 1 -count=1 ./...` without `TEST_DATABASE_URL` (unit and route/OpenAPI contract). `go vet -p 1 ./...`, `go build -p 1 ./...`, `pwsh -NoProfile -File scripts/verify-docs.ps1`, `git diff --check`.
3. Fresh **disposable** isolated PostgreSQL 17 with `TEST_DATABASE_URL`: migration 000081, replay, CoW address lifecycle and concurrency tests, foreign CPO/customer hiding, active entitlement gate, no-op audit, optimistic update conflict, snapshot no-alias.
4. Confirm only intended artifacts and migrations are added; release is a separate normal non-force commit and a separate deployment decision.

## FE-facing contract

See `docs/contracts/api/cow-customer-service-addresses.md`. Do not add Run 2A UI that implies an operational CoW job or HAL action is available.
