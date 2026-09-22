# WI-20260922-customer-ratings-rehost

Status: Complete (rehosted; release documentation reconciled)
Owner: Codex
Collaborators: Anubhab Dey (CPO/backend boundary owner)
Started: 2026-09-22
Completed: 2026-09-22
Last updated: 2026-09-22

Development-plan reference: Customer ratings / User App network projections
Detailed-plan reference: `docs/work/archive/WI-20260921-customer-session-ratings.md` and `docs/work/archive/WI-20260921-customer-charger-rating-aggregate.md`
Issue/PR reference: None

## Outcome

Rehosted the accumulated customer session-rating and customer charger-rating
aggregate commits after checking the full delta from the last hosted source,
release prerequisites, and runtime environment; reconciled the host record and
prepared the deployment evidence for publication to `main` as requested.

## Scope

- Full source delta from hosted revision `64504d5` through `9e2940c`.
- Build, focused and broad Go verification, environment/migration/worker/mail
  preflight, guarded binary replacement/rehost, post-rehost live verification,
  and deployment documentation.

## Non-goals

- No database migration or dump; the delta has no schema change.
- No new environment variables, Caddy change, mail mutation, frontend code,
  physical charger validation, or HAL change.
- No force push or unrelated changes.

## Claimed surfaces

- Release verification and `evcmsnew-dev.service` runtime.
- `docs/guides/operations/dev-hosting.md`, `docs/PROJECT_STATE.md`,
  `docs/AI_CHANGELOG.md`, and this release record.

## Dependencies and blockers

- PostgreSQL integration tests require an explicitly selected disposable
  `TEST_DATABASE_URL`, which was not set on this host.
- `pwsh` was unavailable for `scripts/verify-docs.ps1`.

## Contract and data impact

The source commits add User App session-owned rating GET/PUT and a
tenant-scoped charger aggregate. No additional contract/source change was
needed for rehost documentation. No live data or migration was changed.

## Verification

- Focused `src/customerauth` and `src/cpo` tests, route/OpenAPI parity,
  `go test -p 1 ./...`, `go vet -p 1 ./...`, `go mod verify`, production build,
  and `git diff --check` passed.
- Live migration 71 is current; seven required/current workers are HEALTHY;
  all 518 mail outbox rows are SENT. `.env` is mode 0600 and contains the exact
  64 `.env.example` key names with no blank, missing, or extra entries.
- Rehosted 2026-09-22: source `9e2940c`; binary SHA-256
  `85fcdad6285df7759f0438ab1949cb701b3b74f2a38d44473dd0c2925ab8af7a`;
  previous binary is retained at
  `/root/evcmsnew-backups/pre-customer-ratings-9e2940c-20260922T102358+0530/evcmsnew`.
- PID 218638 is active with zero restarts and matching process/install hashes.
  Loopback and HTTPS live, ready, docs, and OpenAPI return 200; live OpenAPI
  has 249 operations; unauthenticated rating requests return 401; Caddy
  validates; no new-process error/panic/fatal logs or new SENT mail rows were
  observed.
- PostgreSQL-gated lifecycle tests, PowerShell docs verification, physical
  OCPP, and SMTP acceptance remain unverified for the reasons above/out of
  scope.

## Handoff

Keep ratings tied to completed customer-owned sessions and preserve the
session/charger/hub identity frozen when the row is first created. Charger
aggregates use only current-CPO session-owned overall ratings on already
authorized charger projections.

## Completion

Rehost, runtime verification, and documentation reconciliation are complete.
The evidence is included in the authorized `main` update. Remaining
unverified boundaries are listed above.
