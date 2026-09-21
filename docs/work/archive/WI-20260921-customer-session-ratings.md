# WI-20260921-customer-session-ratings

Status: Complete
Owner: Codex
Started: 2026-09-21

## Outcome

Expose one customer-owned, charging-session rating resource through the User
App boundary without changing the existing CPO rating read surface.

## Invariants

- Identity is CPO + customer + completed charging session; the customer API
  never accepts tenant, customer, charger, or hub identifiers.
- The existing partial unique `(cpo_id, session_id, customer_id)` index for
  non-null session IDs is the concurrent-write authority.
- Only durable `COMPLETED` sessions are rateable. A foreign/unowned session is
  indistinguishable from absent. Existing rating reads do not reapply write
  eligibility.
- PUT creates or replaces mutable rating fields only. It never creates history
  or direct charger/hub reviews. Optional dimensions are part of session
  feedback and omitted fields clear on replacement.

## Claimed surfaces

- `src/customerauth`, existing customer-rating persistence/model, User App
  OpenAPI and frontend handoff, schema/project state/plan/changelog, focused
  tests, and PostgreSQL integration coverage.

## Non-goals

No migration, delete/moderation/reply/aggregate/public rating surface, CPO or
platform mutation, mail/event/HAL work, frontend implementation, deployment,
or backfill.

## Verification

- Focused customer-rating validation, strict-body, and app-ID boundary tests;
  full `./src/customerauth` and `./src/cpo` packages; OpenAPI/runtime route
  parity; and documentation contract verification passed.
- PostgreSQL lifecycle and concurrent-first-PUT coverage is present but skipped
  in this checkout because `TEST_DATABASE_URL` is unset. It must run only
  against an explicitly selected disposable PostgreSQL database.
