# Manual Platform Subscriptions

Status: In Progress

## Objective

Restore the platform subscription catalog, immutable published plan versions,
and CPO subscription history without restoring platform billing or inventing a
provider-driven lifecycle.

## Implemented Slice

- Migration twelve restored six tables; forward migration thirteen returns the
  two dormant entitlement tables to `retired_commercial`. Four subscription
  catalog/history tables remain active and five billing tables stay retired.
- Platform-superadmin-only HTTP commands manage plans, issue/renew/change a
  CPO subscription, explicitly transition status, and inspect history.
- UUIDs, plan version numbers, timestamps, audit records, events, and
  idempotency records are server-generated.
- There is no payment provider, invoice/payment route, payment webhook,
  automatic renewal, scheduled plan change, automatic trial completion,
  automatic expiration/cancellation, worker, or subscription email.
- CPO activation/suspension remains an independent platform decision.
- Feature keys and entitlement overrides are not configured, exposed, or
  enforced until a defined module catalog and server-side gates are approved.

## Acceptance Criteria

- Published plan snapshots cannot be mutated.
- A current CPO subscription is unique and every manual transition is auditable
  and safely retryable with its idempotency key.
- Period boundaries do not alter state without a superadmin command.
- Billing routes/tables/workers remain out of the active runtime.
- OpenAPI, human contracts, frontend guidance, realtime documentation, and
  route parity describe exactly the manual boundary.

## Verification

- PostgreSQL migration up/down/data-preservation and manual lifecycle test with
  a disposable `TEST_DATABASE_URL`.
- Route authentication and OpenAPI/runtime parity.
- Documentation verification, `go test ./...`, `go vet ./...`, and
  `git diff --check`.
