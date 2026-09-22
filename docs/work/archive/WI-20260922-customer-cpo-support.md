# Customer-to-CPO durable support workflow

Status: complete (source-verified; PostgreSQL runtime gate skipped)

## Objective

Add the authenticated User App customer-to-own-CPO support workflow without
changing the existing CPO-to-Platform support contract or exposing the new
channel to Platform support.

## Claimed surfaces

- `src/support/` shared support domain, HTTP registration, tests
- support persistence models and additive migration 72
- CPO permission catalog/default roles
- customer-auth route composition and support authentication boundary
- durable mail template catalog, frontend action-link configuration
- OpenAPI, API/schema/mail/configuration/front-end handoffs, project records

## Invariants

- `CPO_PLATFORM` and `CUSTOMER_CPO` tickets are isolated at every query and
  mutation boundary.
- A customer identity is always derived from the authenticated customer
  principal and is never represented as an administrative user.
- CPO customer-support access is tenant-scoped and capability-gated; explicit
  DENY continues to win.
- Ticket mutation, immutable event, and mail intent commit together; mail
  payloads omit private message bodies.
- Existing CPO-to-Platform routes and Platform support views remain compatible.

## Verification target

Focused support, customer-auth, CPO permission, mail, route/OpenAPI, and
PostgreSQL-gated lifecycle/isolation tests, followed by repository checks.

## Completed source state and verification

- Migration 72, service/routes, channel predicates, recipient selection, and
  focused PostgreSQL-gated customer-support coverage are implemented.
- User App/CPO handoffs, support workflow, contracts, schema, configuration,
  mail-outbox docs, plan, state, and changelog are updated.
- Passed: `gofmt` on changed Go files; focused `./src/support`, `./db`, mail,
  permission, configuration, model, and route/OpenAPI checks; documentation
  verifier; `go test -p 1 ./...`; `go vet -p 1 ./...`; `go build -p 1 ./...`;
  and `git diff --check`.
- OpenAPI/runtime route parity passed and the checked contract contains 258
  operations, including exactly these eight additions.
- The PostgreSQL-gated customer-support and mail-constraint tests were invoked
  with `TEST_DATABASE_URL` absent and skipped with the exact reason
  `TEST_DATABASE_URL is not set`. No development/live database was selected.
- The source slice was subsequently committed and pushed to `main` as
  `48f2ecae43d07dbda30cec673b3c3c4faf985da4`. The original verification did
  not deploy/restart/rehost, execute a development/live migration, contact
  SMTP, or modify HAL. Runtime/deployment acceptance remains externally
  unverified.
