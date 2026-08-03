# Complete Superadmin Control Plane Plan

Status: In Progress

Approved direction updated: 2026-07-31

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
- Search/filter/cursor CPO discovery, mutable business profile, and durable
  lifecycle reason/actor/time
- One durable primary administrator with visibility, replacement/restoration,
  credential-free resend, and CPO administrative-session revocation
- Durable platform audit query
- Durable ordered platform events
- Authenticated SSE plus REST replay
- Worker registration, health views, and readiness degradation
- Platform-maintenance and observed mail-outbox workers
- API documentation toggle and runtime/OpenAPI drift verification
- Canonical SuperAdmin frontend handoff across auth, CPO control, audit,
  workers, replay/SSE, errors, security, verification, and known gaps

Administrative password recovery remains enumeration-safe while the eligible
recipient's encrypted email supplies the recovery ID, code, and expiry required
by reset. It is part of the shared authentication foundation, not a
SuperAdmin-control-plane governance command.

## Retired Prototype

The subscription, entitlement, platform-invoice, and platform-payment modules
are no longer product surfaces. Because migrations seven and eight reached the
development VPS, migration nine preserves their tables in
`retired_commercial` instead of deleting data. It disables the retired worker
records and blocks retirement while related mail is pending. No active route,
model, worker, or OpenAPI operation uses those records.

## Remaining Implementation Slices

### 1. CPO lifecycle and first-admin recovery

Status: Implemented

This is the current implementation slice and the complete Superadmin dependency
for CPO onboarding and access:

- search/filter/cursor-paginate CPOs by business identity, lifecycle, and app-ID
  mode, using a stable newest-first cursor;
- edit the mutable CPO business profile without changing its stable slug,
  platform-owned lifecycle, or app identity;
- require GSTIN plus complete address fields for creation/profile replacement,
  retain normalized uniqueness for GSTIN and slug, and expose an authenticated
  non-reserving slug-availability preflight for the creation form;
- require a bounded human reason for activation and suspension and retain the
  current reason, actor, and transition time on the CPO;
- expose one durable primary-administrator designation per provisioned CPO;
- restore the current primary administrator or replace it with a new/existing
  active identity without overwriting an existing password;
- revoke the replaced administrator's CPO-scoped sessions and refresh tokens in
  the same transaction;
- resend onboarding as a safe credential-free recovery message. It directs the
  recipient to normal password recovery instead of regenerating or disclosing a
  global identity password;
- expose the latest correlated onboarding-mail delivery metadata without
  decrypting payloads;
- explicitly revoke all active administrative sessions for one CPO;
- continue revoking all tenant sessions when the CPO is suspended;
- write state, audit evidence, durable realtime events, and applicable encrypted
  mail in one PostgreSQL transaction;
- preserve tenant data and already-started operational recovery paths.

Compatibility:

- the existing CPO collection retains its `cpos` field and adds cursor metadata;
- lifecycle commands now require a JSON reason, which is a deliberate contract
  change before the Superadmin frontend is integrated;
- stable CPO IDs, slugs, app IDs, authentication scopes, and tenant boundaries
  are unchanged.
- registration/profile clients must now send GSTIN, address, city, state, and
  pincode; this is an intentional validation and persistence contract change.

Acceptance criteria:

- a frontend can complete CPO creation, discovery, profile maintenance, manual
  access control, app-ID transition, primary-admin recovery, onboarding resend,
  and administrative-session invalidation from documented APIs alone;
- repeated lifecycle requests for an already-matching state do not create a
  second state transition;
- only one membership per CPO is designated primary, including under concurrent
  replacement attempts;
- replacement never overwrites an existing identity's password and never grants
  authority outside the selected CPO;
- no endpoint returns a password, OTP, token, decrypted mail body, or tenant
  integration secret;
- reconnecting clients recover every committed mutation through REST snapshots
  plus ordered platform-event replay.

Verification completed:

- migration ten applied and rolled back on disposable loopback PostgreSQL 17;
- the PostgreSQL lifecycle test covered correlated onboarding mail, list search
  and cursor behavior, profile replacement, idempotent reasoned activation,
  primary-admin replacement, previous-admin session/refresh revocation,
  credential-free resend, and platform-session isolation;
- the then-current 49-operation runtime/OpenAPI surface, documentation
  verification, full Go tests, vet, and diff checks passed.

Current verification limitation:

- The mandatory registration/slug-availability extension has validation,
  migration-content, route/OpenAPI, and full-source coverage. Its migration
  eleven and PostgreSQL constraint/uniqueness lifecycle checks have not run
  because no disposable `TEST_DATABASE_URL` is configured, so this slice is
  `Implemented` pending that database verification.

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

Migration nine and migration ten have completed their separate disposable
PostgreSQL lifecycle checks. Applying migration ten to a non-disposable
database remains a deployment action requiring explicit human approval, backup,
and the documented migration workflow.
