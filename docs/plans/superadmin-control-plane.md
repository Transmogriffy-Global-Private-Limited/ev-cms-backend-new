# Complete Superadmin Control Plane

Status: In Progress

## Objective

Finish the CMS platform-management plane so platform superadmins can provision,
license, support, secure, communicate with, and operate CPO tenants without
receiving silent access to tenant business data.

This plan is the binding implementation map for the feature. It replaces the
legacy CMS pattern where platform administration and tenant operations shared
one unscoped "admin" surface.

## Evidence and Existing State

The new CMS already implements:

- platform authentication, email OTP, encrypted access tokens, and sessions;
- CPO creation, inspection, activation, suspension, and app-ID rotation;
- transactional initial-CPO-admin onboarding;
- an encrypted PostgreSQL mail outbox and retrying SMTP worker;
- privileged audit writes for implemented credential and CPO operations;
- canonical OpenAPI and embedded Swagger UI.

The legacy CMS data model contains identities, admin profiles, tenant-associated
chargers, hubs, wallets, payments, logs, support, disputes, feedback, and
billing jobs, but no durable software-subscription model. Its route names mix
platform and tenant operations and are not a safe contract to preserve.

## Permanent Boundaries

- A CPO may exist with no subscription.
- Platform superadmins manage the platform and tenant lifecycle; they do not
  automatically read or mutate tenant customers, wallets, charging sessions,
  charger state, payment credentials, or other CPO business records.
- Subscription state and CPO lifecycle are independent. Suspension is an
  explicit platform action; subscription expiry does not silently destroy or
  rewrite tenant data.
- Subscription enforcement must never prevent completion, callback ingestion,
  reconciliation, or billing of already-active charging sessions.
- Tenant Razorpay credentials remain CPO-owned and unavailable to platform
  superadmins. Platform subscription billing is provider-neutral until a
  separate platform-billing provider is approved.
- Administrative commands use REST. Realtime announces committed facts and
  supports view invalidation; it is never authoritative state.
- PostgreSQL is authoritative for subscriptions, entitlements, events, mail,
  notifications, worker heartbeats, audit records, and delivery state.
- Durable event and work records are inserted in the same transaction as the
  state change that caused them.
- No endpoint returns OTPs, temporary passwords, token material, decrypted mail
  payloads, provider secrets, or cryptographic keys.

## Surface Map

### Platform overview and system status

- `GET /api/v1/platform/overview`
- `GET /api/v1/platform/system/status`

Overview exposes platform aggregates and operational counts, not tenant
business data. System status exposes only non-secret runtime/configuration
metadata.

### CPO lifecycle and administrator recovery

- existing create, list, get, activate, suspend, and app-ID endpoints;
- paginated/filterable CPO listing;
- `PATCH /api/v1/platform/cpos/{cpo_id}`;
- `POST /api/v1/platform/cpos/{cpo_id}/restore-pending`;
- `POST /api/v1/platform/cpos/{cpo_id}/app-id/regenerate-dummy`;
- list/assign initial or replacement CPO administrators;
- resend onboarding and revoke a CPO administrator's sessions.

Routine CPO staff management remains a tenant-owner/admin responsibility.

### Platform-superadmin governance

- list platform administrators;
- invite or grant verified platform authority;
- deactivate/reactivate authority;
- revoke platform sessions;
- remove authority while preventing removal of the final active superadmin.

Sensitive authority changes require a reason, recent authentication, audit, and
realtime security events.

### Subscription catalog and entitlements

- immutable/versioned plan catalog;
- plan draft, publish, archive, list, and inspect operations;
- exact minor-unit pricing with currency and billing interval;
- structured feature entitlements and numeric limits;
- optional CPO subscription assignment;
- trial, active, paused, past-due, cancelled, and expired lifecycle;
- scheduled effective changes and cancellation-at-period-end;
- immutable subscription history;
- effective-entitlement query for each CPO;
- explicit per-CPO entitlement overrides with reason and expiry.

Plan publication snapshots a version. Existing subscriptions never change
because a draft plan is edited.

### Platform subscription billing records

- billing-account metadata for a CPO;
- immutable invoices and invoice line items;
- manual/provider-neutral payment records;
- payment allocation and invoice status;
- external-provider references and idempotency keys without embedding a
  provider SDK;
- billing timeline and export-safe queries.

Automatic payment collection and provider webhooks remain disabled until a
platform billing provider and credentials are separately approved.

### Audit and security operations

- paginated/filterable platform audit query;
- audit detail with sanitized structured data;
- locked-identity query and explicit unlock;
- user- and CPO-scoped administrative session revocation;
- security event visibility;
- mandatory operator reason for recovery or revocation actions.

### Mail operations

- mail overview;
- paginated/filterable outbox job metadata;
- individual job metadata;
- failed-job retry and unsent-job cancellation;
- immutable attempt history;
- masked recipient by default;
- no arbitrary raw-email endpoint and no payload decryption endpoint.

### Notifications and announcements

- durable platform-admin notification inbox with read/unread state;
- versioned, allowlisted communication templates;
- draft/preview/schedule/publish/cancel platform announcements;
- audience snapshot for all CPO admins, selected CPOs, or CPO lifecycle groups;
- per-recipient delivery records and aggregate progress;
- email as the initial outbound channel;
- realtime notification of operational and announcement state.

Tenant marketing, arbitrary customer segmentation, SMS, and push delivery are
not part of this feature.

### Workers and durable jobs

- durable worker-instance heartbeats;
- healthy, degraded, stale, and disabled states;
- worker/job overview and detail;
- retry/cancel operations only for explicitly recoverable durable work;
- no HTTP endpoint to start, stop, or kill a process;
- readiness degradation for required stale workers;
- stale-claim recovery and at-least-once processing.

### Realtime and event recovery

- durable `platform_events` sequence;
- authenticated SSE stream for platform sessions;
- REST catch-up endpoint using an event cursor;
- `Last-Event-ID` recovery;
- heartbeat frames and session-revocation termination;
- at-least-once delivery, stable IDs, ordered replay, and client deduplication;
- cursor-expired response requiring a REST state refresh;
- event retention and cleanup worker.

Initial event families:

- `platform.cpo.*`
- `platform.subscription.*`
- `platform.invoice.*`
- `platform.mail.*`
- `platform.worker.*`
- `platform.security.*`
- `platform.announcement.*`
- `platform.system.*`

Events contain resource identifiers and safe state-transition metadata, never
secrets or decrypted payloads.

### API documentation control

- `API_DOCS_ENABLED` controls both `/docs` and `/openapi.yaml`;
- enabled and disabled route-registration tests;
- ordinary API routes work in both states;
- OpenAPI remains the authoritative REST contract;
- SSE operations and event envelopes are documented in OpenAPI plus a focused
  realtime contract.

## Persistence

The implementation will add versioned migrations for:

- platform events and retention metadata;
- worker instances and worker/job attempts where not already owned elsewhere;
- subscription plans and immutable plan versions;
- plan entitlements and limits;
- CPO subscriptions, lifecycle history, and overrides;
- CPO billing accounts, invoices, line items, payments, and allocations;
- platform notifications and recipient read state;
- announcement campaigns, audience snapshots, and delivery records;
- mail-attempt history and operator recovery metadata.

Constraints and indexes must enforce:

- at most one current non-terminal subscription per CPO;
- immutable published plan versions;
- exact currency/minor-unit pricing;
- valid lifecycle transitions;
- idempotent external references and operator commands;
- tenant-safe CPO foreign keys;
- one delivery per campaign recipient/channel;
- one platform event ID ordering sequence;
- one live worker heartbeat identity per process instance;
- no removal of the final platform administrator through application
  transactions.

## Realtime Delivery Model

```text
REST command
→ validate platform principal and request
→ authorize operation
→ database transaction
→ state transition + audit + platform event + queued notification
→ commit
→ SSE dispatcher observes committed event
→ connected platform clients receive safe event
→ frontend refreshes authoritative REST resource
```

Reconnect:

```text
client reconnects with Last-Event-ID
→ server validates platform session
→ query durable events after cursor
→ replay in ID order
→ continue live delivery
```

## Subscription Lifecycle

```text
NONE
→ TRIAL
→ ACTIVE
→ PAST_DUE
→ ACTIVE
→ CANCELLED or EXPIRED
```

`PAUSED` is an explicit operator action. Scheduled plan changes apply at a
recorded effective boundary. Cancellation may be immediate or at period end.
Every transition stores actor, reason, previous state, next state, effective
time, and idempotency key.

Effective entitlements are resolved as:

```text
published plan-version snapshot
→ active CPO-specific overrides
→ safe no-subscription baseline
```

The safe baseline permits authentication, account recovery, completion and
reconciliation of existing operations, and access to subscription/billing
status. Product-feature enforcement is added only as each tenant feature is
implemented.

## Implementation Slices

1. API-docs toggle remediation and platform module wiring.
2. Durable platform events, audit query, worker heartbeats, catch-up API, and
   authenticated SSE.
3. Plan catalog, plan versions, entitlements, CPO subscriptions, overrides,
   lifecycle history, mail, audit, and realtime events.
4. Platform billing accounts, invoices, payments, allocations, and billing
   timeline.
5. Complete CPO lifecycle/admin recovery and platform-admin governance.
6. Mail operations, attempt history, worker operations, and security recovery.
7. Notifications, announcements, audience snapshots, delivery worker, and
   realtime progress.
8. Overview, system status, residue scan, full contract completion, and
   end-to-end verification.

Each slice must update migrations, models, services, authorization, routes,
OpenAPI, human contracts, tests, verification, project state, this plan, and
the changelog together.

## Implementation Progress

- Slice 1 implemented and verified: API documentation registration toggle.
- Slice 2 implemented and Go-verified: durable events, audit query, worker
  health/readiness, REST replay, and authenticated SSE.
- Slice 3 implemented and Go-verified: immutable plan versions, CPO
  subscriptions, lifecycle reconciliation, entitlements, overrides, mail,
  audit, and events.
- Slice 4 implemented and Go-verified: provider-neutral billing accounts,
  immutable issued invoice terms/lines, payment allocation/reversal, billing
  timeline, overdue reconciliation, mail, audit, and events.
- Slices 5 through 8 remain pending.
- PostgreSQL execution of migrations 6 through 8 and their lifecycle tests
  remains pending until a disposable `TEST_DATABASE_URL` is explicitly
  selected.

## Acceptance Criteria

- Every platform endpoint requires a validated `PLATFORM` principal unless it
  is an existing authentication or health endpoint.
- A CPO can be created, activated, and operated with no subscription record.
- Superadmin can create/version/publish plans and manage the complete
  subscription lifecycle without accessing tenant secrets or business data.
- Published subscription terms and invoices are immutable and exact.
- Every privileged mutation records audit, durable event, and applicable
  notification work atomically.
- Failed mail and notification delivery is visible and safely recoverable.
- Worker health is based on durable heartbeats, not only process memory.
- Realtime reconnect recovers committed events without making SSE a source of
  truth.
- Revoked platform sessions lose REST and realtime access.
- The last active platform administrator cannot be removed or deactivated.
- OpenAPI and runtime routes agree exactly, including API-docs enabled and
  disabled behavior.
- Focused PostgreSQL lifecycle tests and the full repository verification pass.

## Verification

- migration discovery, up/down pairing, clean up/down/up, and idempotent up;
- constraint and concurrency tests against disposable PostgreSQL;
- subscription lifecycle and entitlement-resolution tests;
- invoice/payment exactness and idempotency tests;
- platform authorization and tenant-boundary route tests;
- mail/notification retry, duplicate, stale-claim, and cancellation tests;
- worker heartbeat degradation/recovery tests;
- SSE replay, ordering, duplicate, cursor expiry, heartbeat, and revoked-session
  tests;
- OpenAPI validation and runtime-operation drift checks;
- API-docs enabled and disabled route tests;
- documentation verification;
- `go test ./...`, `go vet ./...`, `git diff --check`, and residue scans.

## Risks and Decisions

- Automatic subscription payment collection remains provider-neutral until the
  human approves a platform billing provider and credential boundary.
- CPO subscription entitlements must not become a hidden bypass into tenant
  data.
- Realtime volume must remain operationally bounded; routine high-volume tenant
  facts are not platform events.
- Mail and events are at-least-once. Templates and clients must tolerate
  duplicates.
- Retention and cleanup require conservative defaults until production volume
  is observed.
