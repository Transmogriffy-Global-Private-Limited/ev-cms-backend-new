# WI-20260831-mail-outbox-template-catalog

Status: Completed (source and local verification)
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-31
Last updated: 2026-08-31

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — platform operations
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Repair the durable `mail_outbox` template CHECK catalogue so every currently
supported application template, including the support-ticket lifecycle mails,
can be enqueued atomically with its business mutation.

## Scope

- Forward migration pair `000058` for the template CHECK.
- One explicit Go durable-template catalogue, renderer/validation alignment,
  database-gated CHECK regression, support lifecycle coverage, and mail contract
  documentation.

## Non-goals

- Deployment, service restart, live database mutation, SMTP contact, or any
  support API behavior change.

## Claimed surfaces

- `mail_outbox`, `db/migrations`, `db` migration tests, `src/mail`,
  `src/support` integration coverage, and mail/support workflow documentation.

## Dependencies and blockers

- Depends on the existing immutable migration sequence through 000057.
- PostgreSQL execution is deliberately gated on an already-selected disposable
  `TEST_DATABASE_URL`; this work does not create or configure one.

## Contract impact

- Internal durable-mail contract only: current application-supported template
  names become enforceable database values for new and updated outbox rows.
- No HTTP, frontend, SMTP transport, or support API payload changes.

## Data and migration impact

- Adds forward migration 000058. It preserves historical rows accepted by the
  prior stale CHECK and rejects unknown values for future writes.

## Current state

- Migration 000058 now replaces the stale 13-name CHECK with the exact current
  24-name application catalogue for future writes. Its `NOT VALID` form keeps
  historical rows intact while rejecting unsupported inserts and updates.
- Go validation and renderer classification share one explicit catalogue.
  Support status mail intent remains transactional; the regression proves a
  failed required intent rolls the status projection and lifecycle history back.

## Verification

- `go test -count=1 ./db ./src/mail`, `go test ./...`, `go vet ./...`,
  `go build ./...`, OpenAPI/runtime route parity, documentation verification,
  and `git diff --check` pass locally.
- The PostgreSQL migration/constraint and support lifecycle integration cases
  are skipped because `TEST_DATABASE_URL` is unset. No test database was
  created.

## Handoff

- Preserve fail-closed unknown-template validation and transactional outbox
  behavior. The forward migration must not delete historical mail rows; down
  must refuse rollback while newly introduced template rows exist.

## Completion

- Source, documentation, and local non-PostgreSQL verification are complete.
- No migration was applied, and nothing was deployed, restarted, committed,
  pushed, or sent through SMTP.
