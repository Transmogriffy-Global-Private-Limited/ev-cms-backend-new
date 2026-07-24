# Complete Superadmin Control Plane Plan

Status: In Progress

Approved direction updated: 2026-07-24

## Objective

Complete the platform-management surface so a superadmin can provision and
govern CPO access, recover administrators, manage platform administrators,
observe security/mail/workers, communicate with CPOs, and operate the CMS
without accessing tenant business data.

## Permanent Boundaries

- CPO access is manually controlled through `PENDING`, `ACTIVE`, and
  `SUSPENDED` lifecycle state.
- The CMS does not manage tenant subscriptions, plan entitlements, platform
  invoices, or platform payments.
- No payment event automatically activates or suspends a CPO.
- Platform-superadmin authority is separate from CPO membership.
- A platform superadmin does not receive tenant Razorpay secret plaintext or
  unrestricted tenant business-data access.
- PostgreSQL is authoritative. Realtime events announce committed facts and
  support replay; they do not replace REST recovery.
- Privileged recovery and governance actions require an actor, reason, audit,
  and durable event where applicable.

## Implemented Foundation

- CPO create/list/detail/activate/suspend/app-ID replacement
- Generated dummy app IDs and first-admin onboarding
- Durable platform audit query
- Durable ordered platform events
- Authenticated SSE plus REST replay
- Worker registration, health views, and readiness degradation
- Platform-maintenance and observed mail-outbox workers
- API documentation toggle and runtime/OpenAPI drift verification

## Retired Prototype

The subscription, entitlement, platform-invoice, and platform-payment modules
are no longer product surfaces. Because migrations seven and eight reached the
development VPS, migration nine preserves their tables in
`retired_commercial` instead of deleting data. It disables the retired worker
records and blocks retirement while related mail is pending. No active route,
model, worker, or OpenAPI operation uses those records.

## Remaining Implementation Slices

### 1. CPO lifecycle and first-admin recovery

- search/filter/paginate CPOs;
- reasoned activation and suspension;
- restore or replace the first administrator safely;
- resend onboarding for eligible unsent/recoverable cases;
- revoke CPO administrative sessions when access is suspended;
- preserve tenant data and already-started operational recovery paths.

### 2. Platform-superadmin governance

- list platform administrators;
- invite or grant authority through an explicit verified flow;
- deactivate or remove authority without deleting the global identity;
- prevent removal of the last active platform administrator;
- revoke affected platform sessions transactionally;
- audit every authority change.

### 3. Security operations

- paginated/filterable platform audit detail;
- locked-identity query and explicit reasoned unlock;
- user- and CPO-scoped administrative session revocation;
- security-event visibility without exposing tokens, OTPs, or secret payloads.

### 4. Mail operations

- mail overview and filtered job metadata;
- individual job metadata without decrypted bodies;
- retry failed jobs and cancel eligible unsent jobs;
- template-level delivery metrics;
- bounded retention/reconciliation.

### 5. Notifications and announcements

- platform-owned notification records;
- CPO-targeted and platform-wide announcements;
- explicit audience snapshots;
- durable delivery/retry state;
- REST recovery and realtime invalidation;
- no tenant-business-data payloads.

### 6. Overview and system status

- bounded aggregate CPO/access/session/mail/worker counts;
- service/database/worker state;
- current deployment/version metadata where safely available;
- no unbounded tenant-data aggregation.

## Realtime Contract

```text
state-changing transaction
→ durable platform event
→ authenticated SSE
→ client deduplication by event ID
→ REST refresh
```

Connection requirements:

- platform bearer authentication;
- heartbeat;
- cursor replay;
- bounded retention and explicit cursor-expiry recovery;
- revoked sessions lose access;
- no secret or tenant-business payloads.

## Implementation Order

1. Retire subscription/platform-billing runtime and contracts safely.
2. Complete CPO lifecycle and first-admin recovery.
3. Complete platform-administrator governance.
4. Complete security and mail operations.
5. Add notifications and announcements.
6. Add overview and system-status queries.
7. Complete residue, recovery, concurrency, and operational verification.

## Acceptance Criteria

- Superadmins can grant and remove CPO access manually without commercial
  records.
- Retired subscription/billing routes return `404` and are absent from
  OpenAPI/Swagger.
- Retired tables are preserved outside the runtime schema and no worker keeps
  readiness unhealthy.
- Privileged actions are authenticated, authorized, auditable, and tenant-safe.
- Realtime loss is recoverable through cursor replay or REST refresh.
- Documentation alone describes the implemented and remaining control plane.

## Verification

- migration discovery, archive/restore, and no-drop assertions;
- route/OpenAPI bidirectional coverage;
- explicit retired-route absence tests;
- platform authorization and tenant-boundary tests;
- worker readiness after retirement;
- mail pending-job retirement guard;
- documentation drift verification;
- `go test ./...`;
- `go vet ./...`;
- `git diff --check`.

Disposable PostgreSQL execution of migration nine must be completed before it
is applied to a non-disposable database. The development deployment must not be
updated if the pending-mail guard fails; an operator must inspect and resolve
those jobs deliberately.
