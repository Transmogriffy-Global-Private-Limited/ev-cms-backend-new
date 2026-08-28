# CPO Access, Mail, Subscription, and Support Completion Plan

Status: Implemented and Verified; deployed in runtime revision `342d65a`

## Objective

Close the current product gaps without changing CMS/HAL ownership: CPO staff
decisions use a source-controlled capability catalog with live membership
state, mail remains a durable outbox but renders semantic typed templates,
subscription lifecycle facts enqueue deduplicated notices, and support becomes
an auditable customer-service workflow.

## Invariants

- CPO authority is derived from the active membership for the authenticated
  user and trusted CPO scope on every decision.
- `DENY` overrides `ALLOW`, which overrides role defaults; unknown or missing
  state denies access. Roles are named default bundles, not runtime bypasses.
- Mail is enqueued transactionally and delivered only by the existing worker;
  no action is acknowledged merely because a mail notification was requested.
- Support bodies are visible only to the authorized CPO tenant or platform
  support actor. History records state transitions and mutation identity.

## Slices

1. Define the complete permission registry, one reusable fresh evaluator, and
   migrate CPO/support/integration route enforcement from broad ADMIN gating.
2. Add membership effective-permission discovery, whole-set override mutation,
   primary/delegation safeguards, and focused authorization tests.
3. Introduce typed semantic mail templates, validated frontend links, display
   timezone handling, and migrate every current producer.
4. Enqueue subscription warning/expiry notifications idempotently from the
   lifecycle transaction without coupling expiry to SMTP delivery.
5. Add support lifecycle history, bounded list/detail/history contracts,
   transition graph, idempotent mutations, secure request parsing, and mail
   notifications.
6. Update migrations, OpenAPI, frontend/integration guidance, project state,
   changelog, and run focused then broad verification.

## Verification

Use focused tests for precedence, cross-tenant membership freshness,
delegation, mail template/action-link rendering, lifecycle deduplication, and
support transitions before repository-wide Go, documentation, and build checks.
Disposable PostgreSQL tests are explicitly conditional on `TEST_DATABASE_URL`.
