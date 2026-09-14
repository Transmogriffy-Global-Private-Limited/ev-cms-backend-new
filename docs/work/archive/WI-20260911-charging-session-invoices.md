# WI-20260911-charging-session-invoices

Status: Complete
Owner: Codex
Collaborators: Anubhab Dey (product and CMS/HAL boundary owner)
Started: 2026-09-11
Last updated: 2026-09-14 (deployment, runtime verification, and documentation reconciliation complete)

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — charging lifecycle and commercial completion
Detailed-plan reference: This work item (approved task handoff)
Issue/PR reference: None

## Outcome

After a charging session reaches durable commercial finality, CMS eventually and
idempotently issues one immutable, numbered, integrity-verifiable customer
invoice PDF. The invoice remains independently downloadable by its customer
and CPO. Automatic invoice delivery has its own truthful state machine and
never changes charging or settlement truth.

## Scope

- Forward migration, durable invoice aggregate, CPO financial-year allocator,
  issuance snapshot, controlled local PDF storage, renderer, and reconciliation
  worker.
- Customer/CPO invoice metadata and authorized download routes; additive
  charging-session/transaction projections.
- Attachment-aware invoice mail delivery with safe retry only before crossing
  SMTP, an explicit ambiguous state after an SMTP outcome cannot be proven,
  and an audited, duplicate-confirmed CPO recovery path.
- Invoice worker health, CPO branding validation/fallback, OpenAPI, frontend
  handoffs, contracts, project memory, focused tests, and source verification.

## Non-goals

- HAL changes, charging/settlement-policy changes, platform billing, S3,
  frontend implementation, deployment, live migration, or SMTP send.

## Claimed surfaces

- `db/migrations/000069_*`, `src/models`, `src/invoice`, `src/customerauth`,
  `src/cpo`, `src/mail`, `src/config`, `main.go`, OpenAPI/contracts/docs, and
  focused tests.

## Dependencies and blockers

- Charging finality is source-confirmed as `COMPLETED + SETTLED`. A Payment is
  optional: zero-amount settled sessions have no Payment row.
- No disposable `TEST_DATABASE_URL` is configured, so PostgreSQL integration
  tests must remain skipped unless the environment changes.

## Contract impact

- Add owner/CPO-scoped invoice download operations and small stable invoice
  lifecycle metadata to historical customer/CPO session projections.
- Generation and email delivery are separate public dimensions. No route leaks
  storage paths, source snapshots, invoice mail internals, or tenant data.

## Data and migration impact

- New invoice aggregate and CPO/Indian-financial-year sequence. One canonical
  invoice is enforced per session; allocation never uses `MAX + 1`, is capped
  at `999999`, and fails explicitly when exhausted.
- `charging_sessions.settled_at` records the real CMS financial-finality
  transition. Existing settled rows are intentionally not backfilled with the
  migration timestamp.

## Current state

- Current `main` and `anubhab-work` are `6cbd782`; worktree was clean before
  this item. Fetched contributor branch was preserved untouched.
- The worker selects only `COMPLETED + SETTLED` sessions. Issuance locks and
  rechecks the session, captures a typed immutable snapshot, allocates the
  serial atomically, and creates a deterministic artifact path. A matching
  crash-window file is adopted; different bytes are never overwritten.
- `READY` integrity failure is terminal `CORRUPT`: download marks it visible,
  delivery marks its work terminal, and reconciliation never regenerates it.
- PNG/JPEG logos are bounded, decodable, stored under a controlled private
  root, snapshotted content-addressably, and deduplicated. Unsafe/missing
  legacy paths safely omit branding.
- Canvas with embedded OFL Noto Bengali and Devanagari fonts is the current
  pure-Go renderer; snapshot and renderer versions dispatch explicitly.
- Customer summaries expose no delivery state. CPO summaries batch-load safe
  delivery state. Customer/CPO downloads use the owner/tenant session UUID.
- Automatic email rollout compares `settled_at` with its durable cutoff. SMTP
  disabled at issuance creates no stale pending work. Preparation failures may
  retry; post-SMTP failures become `AMBIGUOUS`, not automatic resends. CPO
  recovery requires explicit duplicate confirmation and writes an audit row in
  the same transaction as its durable queue change.

## Verification

Already completed before the final six source corrections: OpenAPI runtime
route parity, documentation verification, and the earlier focused invoice
test. They were not rerun because the final corrections did not alter routes
or contracts.

Final source-correction coverage passed on 2026-09-14:

- Focused config validation, invoice lifecycle/projection/download/snapshot,
  and CPO upload-logo regression tests.
- `go test -p 1 ./...`
- `go vet -p 1 ./...`
- `go build -p 1 ./...`
- `git diff --check`

`TEST_DATABASE_URL` is unset. Disposable PostgreSQL lifecycle and concurrency
tests were not run. Migration `000069` was instead applied to the explicitly
targeted development database after a validated mode-0600 backup; the live
ledger and invoice state are recorded below.

## Deployment verification (2026-09-14)

- Applied migration `000069_add_charging_session_invoices`; retained the
  pre-migration dump at
  `/root/evcmsnew-backups/devevcmsnew-before-000069-20260914T110817+0530.dump`
  (SHA-256
  `d9619ffc18615f7af1e7e80ddc6268171f44d87349c1c93246bb50e0a41f7a26`).
- Rehosted source revision `367e2f5`; active binary SHA-256 is
  `7c4fd89d00e0f19e97594017453aa9f0fbe78a1770cad84c23b09facab86a1a8`.
  Prior binary remains at
  `/root/evcmsnew-backups/pre-invoice-367e2f5-20260914T111017+0530/evcmsnew`.
- Configured `/var/lib/evcmsnew/invoices` as the private storage root and added
  only that directory to the systemd writable paths, preserving
  `ProtectHome=read-only`. All 70 invoice artifacts reached READY; every file's
  PDF signature, size, SHA-256, and `0600` mode matched PostgreSQL. Required
  current workers are HEALTHY; loopback/public health, readiness, Swagger,
  245-operation OpenAPI, and Caddy validation passed.
- No invoice deliveries were queued for historical sessions with unknown
  `settled_at`; no SMTP message was sent. SMTP attachment delivery and
  PostgreSQL-gated integration/concurrency tests remain unverified.
- `pwsh` is unavailable, so `scripts/verify-docs.ps1` could not be run.

## Handoff

The invoice pipeline is downstream-only. A failure in discovery, snapshot,
rendering, storage, download, email, or worker health must not alter a session,
wallet hold, wallet ledger, payment, or HAL truth.

## Completion

Source implementation was previously published in commit `ab8d125`. The
development deployment and its documentation are now verified; this work
item's deployment record will be published in the current authorized `main`
update.
