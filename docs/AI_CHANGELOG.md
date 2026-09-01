# AI Changelog

## 2026-09-01 - Complete CPO capability-authority coherence in source

- Made the documented CPO capability, evaluated against active membership and
  matching `X-CPO-App-ID`, the endpoint authority. Roles remain
  source-controlled default bundles; explicit `DENY` wins over every default.
  Evaluator/infrastructure failure now remains a safe `500` rather than being
  misreported as a permission denial.
- Corrected support create/reply ordering so create does not fail after commit
  through an unrelated read check, and reply proves both `support.read` and
  `support.reply` before mutation. Integration reads use `settings.read`; PUT
  and DELETE use `settings.manage`. CPO protected responses use shared
  `no-store` middleware.
- A real staff role change now transactionally revokes only that member's CPO
  administrative sessions and refresh tokens; no-op and override-only changes
  retain sessions. CPO operational SSE streams continue to re-evaluate fresh
  `chargers.operations` authority at heartbeat.
- Repaired CPO OpenAPI security as one Bearer-plus-App-ID AND requirement,
  including the previously omitted hub-GST and user-group membership deletion
  operations, and updated the CPO contracts, frontend handoffs, project state,
  development plan, and documentation verifier to describe capabilities as
  endpoint authority.

Verification: focused auth/permission/middleware/CPO/support/integrations tests,
OpenAPI/runtime route regressions, documentation verification, `go test ./...`,
`go vet ./...`, `go build ./...`, and `git diff --check` pass locally.
PostgreSQL-backed lifecycle cases are skipped because no disposable
`TEST_DATABASE_URL` is configured. No migration, database mutation, deployment,
commit, or push occurred.

## 2026-08-31 - Deploy User App realtime and mail-outbox reconciliation

- Applied migration 058 after a mode-0600 database backup, rebuilt source
  revision `320d489`, and rehosted the development CMS. The release includes
  authenticated User App full-state realtime projections, CPO stream ordering,
  and the reconciled durable mail-template catalogue.
- Retained the previous binary at
  `builds/evcmsnew.pre-deployed-632ec13-20260831-141003` and the pre-migration
  dump at `/root/evcmsnew-backups/devevcmsnew-before-000058-20260831-140835.dump`.

Verification: migration state and the `NOT VALID` mail constraint were checked;
the service is active with zero restarts; loopback/public live and readiness,
OpenAPI (217 operations), Swagger UI, Caddy validation, and post-rehost logs
pass. Focused route, realtime, mail, support, and migration tests plus the
previously completed broad Go tests/vet/build passed. `pwsh` and a disposable
`TEST_DATABASE_URL` are unavailable, so the documentation PowerShell verifier
and PostgreSQL-gated integration tests were not run; physical charger and SMTP
delivery acceptance remain unclaimed.

## 2026-08-31 - Reconcile the durable mail-outbox template catalogue in source

- Added forward migration 058 to replace the stale `mail_outbox` template
  CHECK with the 24 templates that current source validation and rendering
  support. The migration preserves preexisting historical rows without deleting
  or rewriting them, while PostgreSQL rejects unsupported new/updated rows.
- Added one explicit Go catalogue, validation/renderer alignment tests, a
  PostgreSQL-gated constraint regression, and support status rollback coverage
  so required support mail intent cannot be silently lost from the business
  transaction.

Verification: source-focused mail, support, and migration tests plus broad Go
verification are recorded with this slice. PostgreSQL integration coverage
remains separately gated unless an explicitly selected disposable
`TEST_DATABASE_URL` is provided. Migration 058 is now applied on the
development database and the release is deployed in the entry above.

## 2026-08-31 - Correct User App realtime state delivery in source

- Added complete authenticated User App SSE projections for the current
  customer live-session collection and one customer-visible charger's current
  availability. Both reuse existing customer-safe CMS projections and batch
  `liveops` reads; the durable operational-event table only wakes a replacement
  projection and is not delivered as browser state.
- Added transactional wake-up publication for the CMS-owned customer
  `ACTIVE -> STOP_PENDING` transition, bounded heartbeat reprojection for
  time-derived state, customer/app/visibility revalidation, and OpenAPI/User
  App/realtime contract updates. Corrected CPO live-session SSE to establish
  its event watermark before reading the initial snapshot.

Verification: focused customer-auth, liveops, operational-realtime, CPO, and
route tests; documentation verification; OpenAPI/runtime route parity; full
`go test ./...`; `go vet ./...`; `go build ./...`; and `git diff --check`
passed. PostgreSQL-gated lifecycle cases remain unrun because
`TEST_DATABASE_URL` is not configured. The source is now included in the
deployed release recorded above; no physical charger or SMTP acceptance is
claimed.

## 2026-08-28 - Deploy CPO live-session OCPP transaction identity

- Rebuilt and rehosted source revision `632ec13`, which exposes the durable
  OCPP transaction identifier as `ocpp_transaction_id` in CPO live-session
  snapshots and SSE payloads. No database migration was pending or required.
- Retained the previously active binary at
  `builds/evcmsnew.pre-deployed-d635446-20260828-164259` for rollback.

Verification: service active with zero restarts, loopback and public live/readiness
checks pass, public OpenAPI contains `ocpp_transaction_id`, Swagger UI responds,
Caddy configuration validates, and post-rehost logs show successful startup.
Focused projection and OpenAPI route-parity tests plus the previously completed
full Go tests and vet passed. `pwsh` is unavailable on this VPS, so the
PowerShell documentation verifier was not run; physical charger and SMTP
delivery acceptance remain unclaimed.

## 2026-08-28 - Add live charging financial and usage projections

- Added one shared snapshot-pricing evaluator for current accrued amounts,
  tenant-scoped CPO customer usage projections, and customer/CPO live-session
  financial projections. Projected amounts remain distinct from final
  settlement totals and use persisted tariff/tax snapshots.
- Extended the existing customer session, CPO customer, and CPO live-session
  contracts and OpenAPI schemas without adding a migration or a second source
  of truth.

Verification: focused projection and commercial tests, the OpenAPI route-parity
test, full `go test ./...`, `go vet ./...`, and `git diff --check` pass. Runtime
revision `d635446` was rebuilt and rehosted without a migration; service/readiness,
public routing, Swagger/OpenAPI (213 operations), Caddy, binary identity, and
post-rehost logs pass. Physical charger, SMTP delivery, disposable PostgreSQL,
and `pwsh` documentation verification are not claimed or unavailable.

## 2026-08-28 - Fix CPO customer aggregate wallet reads

- Split the customer aggregate query into independent session and wallet
  reads, preserving zero-valued usage/session totals while reliably returning a
  customer's wallet balance when no wallet row exists.

Verification: focused CPO aggregate tests, full `go test ./...`, `go vet
./...`, and `git diff --check` pass. Runtime revision `b6b723d` was rebuilt and
rehosted without a migration; service/readiness, public routing,
Swagger/OpenAPI (213 operations), Caddy, binary identity, and post-rehost logs
pass. `pwsh`, `TEST_DATABASE_URL`, and physical charger acceptance remain
unavailable/unclaimed.

## 2026-08-28 - Complete the durable support-ticket core

- Added support migration 057, immutable ticket lifecycle events, the final
  `OPEN`/`IN_PROGRESS`/`RESOLVED`/`CLOSED` transition graph, locked status and
  reply mutation paths, truthful CPO reopen history, and reply idempotency.
- Replaced unbounded full-thread list responses with cursor-paginated queue
  summaries; detail retains ordered messages and history. Added strict JSON
  request boundaries, OpenAPI schemas/filters/conflict responses, and the
  source support workflow contract.

Verification: focused support/mail/auth/permission tests, OpenAPI/runtime route
parity, full `go test ./...`, `go vet ./...`, and `git diff --check` passed.
Migrations 56 and 57 were applied after a transactional dry-run and a database
backup at `/root/evcmsnew-backups/devevcmsnew-before-000056-000057-20260828-103844.dump`.
The migration ordering defect that rejected existing `PENDING` tickets was
corrected before application. Runtime revision `342d65a` was rebuilt and
rehosted; service/readiness, public routing, Swagger/OpenAPI (213 operations),
Caddy, binary identity, and post-rehost logs pass. SMTP delivery and physical
charger acceptance are not claimed. `pwsh` and `TEST_DATABASE_URL` are
unavailable on this VPS.

## 2026-08-26 - Separate customer execution limits from wallet safety

- Refactored the existing customer start path so customer ENERGY, TIME, and
  MONEY intent is not rejected merely because it differs from the tariff
  billing dimension. The tariff calculator independently supplies only the
  wallet-safe billed-dimension bound; no charger-power conversion is used.
- Added durable energy/duration provenance, CMS migration 055, HAL request
  fields, OpenAPI, user-app and integration contracts, and all 12 customer
  limit x tariff regression cases. Final settlement remains actual frozen-term
  billing; the existing one-time metered buffer is only an overshoot margin.

Verification: focused customer-auth admission matrix, route/OpenAPI parity,
full `go test ./...`, `go vet ./...`, and `git diff --check` pass. Migration 55
was applied after a transactional dry-run and a database backup at
`/root/evcmsnew-backups/devevcmsnew-before-000055-20260826-150844.dump`.
Runtime revision `e8ff810` was rebuilt and rehosted; the enabled service,
loopback/public readiness, Swagger/OpenAPI (211 operations), Caddy
configuration, binary identity, and post-rehost startup logs pass. No physical
charger acceptance is claimed. `pwsh` and `TEST_DATABASE_URL` are unavailable
on this VPS.

## 2026-08-26 - Add customer-selected charging execution limits

- Added optional ENERGY, TIME, and MONEY start limits to the existing customer
  start-intent/wallet-hold/HAL-command path. AUTO now derives only the
  applicable wallet-backed threshold; it no longer invents a one-hour limit.
- Persisted immutable selection metadata and added migration 054. Metered
  holds reserve the configured wallet buffer so normal meter/stop interval
  overshoot is covered before settlement's existing conservative guard.
- Updated the CMS-to-HAL request, OpenAPI, User App handoff, model mapping,
  and targeted admission tests.

Verification: focused customer-auth, HAL client/ops, model, and OpenAPI route
tests, full `go test ./...`, `go vet ./...`, and `git diff --check` pass.
Migration 54 was applied after a transactional dry-run and a database backup at
`/root/evcmsnew-backups/devevcmsnew-before-000054-20260826-111224.dump`.
Runtime revision `a085f29` was rebuilt and rehosted; the enabled service,
loopback/public readiness, Swagger/OpenAPI, Caddy configuration, binary
identity, and post-rehost startup logs pass. No physical charger acceptance is
claimed. `pwsh` and `TEST_DATABASE_URL` are unavailable on this VPS.

## 2026-08-26 - Document the complete SuperAdmin support desk

- Added the canonical SuperAdmin support-desk workflow covering actor scope,
  ticket/message persistence and ordering, exact reply/status behavior,
  request/response examples, errors, ambiguous-write recovery, privacy, and
  all currently unsupported support features. Linked it from the SuperAdmin
  handoff, control-plane concept, shared administrative contract, and docs
  index.
- Corrected the CPO support OpenAPI security requirements from an accidental
  bearer-or-app-ID declaration to the runtime-required bearer-and-app-ID
  declaration. Tightened the support schemas with required response fields and
  actual request length constraints. No routes, database state, or runtime
  behavior changed.

Verification: `scripts/verify-docs.ps1`, focused OpenAPI/runtime route parity,
serial `go test -p 1 ./...`, serial `go vet -p 1 ./...`, and `git diff --check`
pass. No deployment or live database action occurred.

## 2026-08-25 - Add CPO live-session display context

- Added `duration_seconds`, `customer_name`, and `connector_id` to each CPO
  live-session SSE/JSON snapshot row. Duration is an integer measured from the
  session start to the response `as_of` timestamp; the FE can advance its
  display between frames without relying on an ambiguous formatted duration.
- The read now preloads the same-CPO customer relation. It deliberately exposes
  only the customer's display name—not customer ID, email, phone, wallet, or
  commercial data—and keeps `connector_id` as the canonical CMS UUID distinct
  from the connector's physical number.

Verification: focused CPO projection/SSE tests, documentation verification,
OpenAPI/runtime route parity, `go test ./...`, `go vet ./...`, and
`git diff --check` pass. Runtime revision `e6c0ebb` was rebuilt and rehosted
without a migration or database mutation. The enriched snapshot fields,
service readiness, public routing, Swagger/OpenAPI, Caddy, binary identity,
and post-rehost startup checks passed. The prior binary is retained for
rollback. `pwsh` is unavailable on this VPS, so the PowerShell documentation
verifier was not rerun here.

## 2026-08-25 - Add CPO analytics period filters

- Added optional `period` (`day`, `week`, `month`, or `year`) and ISO date
  filters to CPO analytics while keeping charger/connector counts overall.
  The OpenAPI contract now documents the query parameters and the invalid-
  request response; the date example is quoted as an OpenAPI string.
- No migration or data repair was required; analytics reads remain tenant-
  scoped and aggregate the existing charging-session data.

Verification: focused CPO analytics and route/OpenAPI checks, full Go tests,
vet, `git diff --check`, live readiness, protected analytics route, Swagger,
raw OpenAPI (210 operations), Caddy, binary identity, and post-rehost startup
checks passed. Runtime revision `f432f45` was rebuilt and rehosted. `pwsh` is
unavailable on this VPS, so the PowerShell documentation verifier was not
rerun here.

## 2026-08-25 - Make the CPO live-session primary route full-snapshot SSE

- Changed `GET /api/v1/cpo/operations/live-sessions` from a JSON table read to
  the CPO ADMIN/app-ID-scoped SSE that a frontend consumes directly. It emits
  the complete CMS `LiveChargingSessionListResponse` immediately as `snapshot`
  and after committed session, meter, or SoC changes as `live_sessions`.
  The durable event log remains an internal change detector, not browser state.
- Moved the JSON table read to
  `GET /api/v1/cpo/operations/live-sessions/snapshot` for explicit recovery and
  keyset pagination. The filtered event cursor remains advanced reconciliation;
  the old realtime stream is a deprecated compatibility alias of the new full
  snapshot stream. The contract now has 210 operations.
- Updated the CPO API contract, realtime contract, HAL capability manual, and
  CPO frontend handoffs so normal UI code replaces the live table per SSE frame
  and reconnects for a fresh snapshot rather than replaying/merging events.

Verification: focused CPO and operational-realtime tests, OpenAPI/runtime route
parity, `go test ./...`, `go vet ./...`, and `git diff --check` pass. Runtime
revision `d3ac043` was rebuilt and rehosted without a migration or database
mutation. The full-snapshot SSE routes, service readiness, public routing,
Swagger/OpenAPI (210 operations), Caddy, binary identity, and post-rehost
startup checks passed. `pwsh` is unavailable on this VPS, so the PowerShell
documentation verifier was not rerun here.

## 2026-08-25 - Fix CPO customer total-usage aggregate mapping

- Corrected both CPO customer usage aggregate scan targets to map
  `TotalUsageKWh` explicitly to the SQL alias `total_usage_kwh`. GORM had
  inferred `total_usage_k_wh`, so the same queries correctly populated
  `session_count` but silently serialized zero usage despite completed sessions
  with nonzero `charging_sessions.total_kwh`.
- The list and single-customer read now share the canonical alias mapping; no
  migration or data repair is required because the energy is already stored in
  the migration-owned `total_kwh` column.

Verification: focused CPO aggregate and route/OpenAPI checks,
`scripts/verify-docs.ps1`, `go test ./...`, `go vet ./...`, and
`git diff --check` pass. Runtime revision `849d80b` was rebuilt and rehosted
without a migration or data repair. The aggregate mapping, service readiness,
public routing, Swagger/OpenAPI, Caddy, binary identity, and post-rehost checks
passed. The prior binary is retained for rollback. `pwsh` is unavailable on
this VPS, so the PowerShell documentation verifier was not rerun here.

## 2026-08-25 - Harden CPO charging-session charger relation fallback

- Hardened the CPO historical charging-session and live-session reads against
  an incomplete nested GORM preload. When the direct session charger is empty,
  the repository now performs one bounded CPO-scoped charger lookup using the
  persisted connector charger key, with the direct session key as a fallback.
  It supplies the existing human-readable charger and hub projection without
  mutating data or inventing an unresolved relation.

Verification: focused CPO regression and route/OpenAPI checks,
`scripts/verify-docs.ps1`, `go test ./...`, `go vet ./...`, and
`git diff --check` pass. Runtime revision `0fd9774` was rebuilt and rehosted
without a migration or database mutation. Its tenant-scoped charger fallback,
loopback/public health and readiness, Swagger, raw OpenAPI, Caddy, binary hash,
and post-rehost startup checks passed. The prior binary is retained for
rollback. `pwsh` is unavailable on this VPS, so the PowerShell documentation
verifier was not rerun here.

## 2026-08-25 - CPO live-session operations and renewed subscription admission

- Added CPO ADMIN/app-ID-scoped `GET /api/v1/cpo/operations/live-sessions` for
  the bounded CMS-projected ongoing-session table, with human-readable
  charger/hub/connector context and independent committed meter/SoC freshness.
  It returns only `ACTIVE`, `STOP_PENDING`, and `RECONCILIATION_REQUIRED`
  sessions and deliberately excludes customer, wallet, tariff, amount, and
  settlement data.
- Added dedicated retained replay and SSE routes filtered to
  `CHARGING_SESSION` invalidations. SSE revalidates bearer session, ADMIN role,
  and app ID at heartbeat; REST remains the authoritative recovery snapshot.
- Corrected customer commercial admission to prefer the PostgreSQL-enforced
  current subscription over terminal history. An older expired row can no
  longer strand a User App after its CPO is renewed; a current expired or
  elapsed subscription retains the existing narrow start/recharge block.

Verification: focused customerauth, CPO, liveops, operational-event, and
route/OpenAPI checks, `go test ./...`, `go vet ./...`, and `git diff --check`
pass. Runtime revision `aa44e4b` was rebuilt and rehosted with no migration or
database mutation. The service is enabled and healthy; loopback/public health,
readiness, Swagger, raw OpenAPI, expected unauthenticated live-session access,
209-operation count, Caddy validation, binary hashes, and post-rehost logs were
verified. `pwsh` is unavailable on this VPS, so the PowerShell documentation
verifier was not rerun here.

## 2026-08-25 - Preserve legacy CPO identity rows during migration

- Changed unapplied migration 53 to install GSTIN, GSTIN-state, and PIN checks
  as `NOT VALID`, preserving three existing legacy CPO rows without rewriting
  them while enforcing the rules for new and updated rows.
- Documented the later `VALIDATE CONSTRAINT` remediation path after an
  authoritative legal-identity correction.

Verification: read-only live preflight identified three state-prefix mismatches;
no live data was modified. The transactional migration dry-run passed, then the
live database applied migrations 49–53. The service was rehosted at revision
`162b3be`; loopback and public health/readiness, Swagger/OpenAPI routes, service
state, enabled state, Caddy validation, binary hashes, constraint state, and
post-rehost logs were verified. `TEST_DATABASE_URL` and `pwsh` remain
unavailable, so disposable-database and PowerShell documentation checks were
not run.

## 2026-08-25 - CPO and SuperAdmin frontend contract reconciliation

- Added a complete CPO frontend integration handoff covering administrative
  authentication, trusted app-ID tenancy, every routed CPO UI area, commercial
  and network invariants, CPO staff-permission limits, operational replay/SSE
  recovery, and tenant-safe error behavior.
- Added an explicit SuperAdmin/CPO frontend boundary and manually reviewed
  matrix for all 65 current platform APIs plus the 12 shared administrative
  auth APIs. The matrix records actual server authority and FE risk treatment:
  every platform API is still enforced by `PLATFORM`, not a newly invented
  granular permission.
- Reconciled authentication and SuperAdmin handoff prose with current source:
  core CPO administration/integration routes require ADMIN, while CPO support
  and notifications accept an active CPO membership plus the verified app ID.

Verification: manual OpenAPI/matrix inventory comparison confirms all 65
platform plus 12 shared administrative-auth operations exactly once;
`./scripts/verify-docs.ps1`, focused OpenAPI/runtime route parity,
`go test ./...`, `go vet ./...`, and `git diff --check` pass. No database,
deployment, or live-service action occurred.

## 2026-08-25 - CPO GSTIN legal-identity validation

- CPO provisioning/profile replacement now verifies Indian GSTIN structure and
  checksum, enforces GSTIN state-code alignment with the CPO registration
  state, requires six-digit Indian PIN codes, and normalizes meaningful
  CPO/admin text. Initial, replacement-primary, and tenant staff administrator
  names use the same validation.
- Added migration 53 as the PostgreSQL backstop for GSTIN checksum/state and
  PIN invariants. It preserves existing legacy rows with `NOT VALID` checks
  while enforcing new and updated rows, and deliberately adds no redundant
  `(gstin, business_name)` index: normalized GSTIN is already globally unique.
  The platform does not claim legal-name ownership validation without an
  authorized GST registry integration.

Verification: focused CPO tests, documentation/OpenAPI validation, route
parity, `go test ./...`, `go vet ./...`, and `git diff --check` pass. The
PostgreSQL migration/direct-write tests remain unrun because
`TEST_DATABASE_URL` is unset.

## 2026-08-25 - Expired subscription customer-command admission

- Extended the existing audited SuperAdmin renewal command so it can reactivate
  an `EXPIRED` CPO subscription after manual payment confirmation. The renewed
  period starts at the renewal time if a supplied start would already be in the
  past.
- An explicitly expired or elapsed current CPO subscription now rejects only
  new User App charging starts and new Razorpay recharge-order creation with
  `403 cpo_subscription_expired`. Start replay, remote stop, HAL fact/recovery,
  settlement, customer reads, and verification of an existing recharge order
  remain available; an absent subscription record retains legacy behaviour.

Verification: focused customer-auth/subscription tests, documentation/OpenAPI
validation, route parity, `go test ./...`, and `go vet ./...` pass. Disposable
PostgreSQL lifecycle/admission tests remain skipped without `TEST_DATABASE_URL`.

## 2026-08-25 - CPO staff authority, announcement targeting, and charging projections

- Added the source-controlled CPO permission catalog, durable normalized
  membership ALLOW/DENY override mapping, non-primary staff lifecycle APIs, and
  active non-ADMIN CPO session validation. Primary-admin replacement remains a
  separate protected workflow.
- Added multi-CPO announcement target/recipient snapshots, fail-closed mail
  template validation, and multipart text/HTML SMTP delivery without replacing
  the encrypted outbox.
- Added the observed `subscription-lifecycle` worker and additive lifecycle
  evidence mapping. It emits exactly-once durable 7/3/1-day threshold records
  and an atomic system expiry transition without suspending a CPO.
- Added source-controlled embedded mail layouts under `src/mail/templates/`
  and durable CPO/platform support ticket and reply surfaces.
- Enriched CPO customer, charging-session, and charger-transaction responses
  with total usage, human-readable charger/hub/connector details, explicit
  CMS/HAL/OCPP identifiers, and settlement/reconciliation status.

Verification: focused package tests, `./scripts/verify-docs.ps1`, OpenAPI route
parity, `go test ./...`, and `go vet ./...` pass. Database migration and
lifecycle behavior remain unverified without a disposable `TEST_DATABASE_URL`.

## 2026-08-25 - CPO session relation fallback deployment

- Session list/detail reads now preload connector-owned charger relations and
  safely hydrate the charger projection when the direct session relation is
  incomplete; no database migration was required.

Verification: focused OpenAPI route parity, full Go tests, vet, diff checks,
local/public readiness, Swagger, OpenAPI, Caddy validation, and the
post-rehost journal scan passed. `pwsh` is unavailable for `verify-docs.ps1`;
disposable PostgreSQL and physical HAL acceptance remain unrun.

## 2026-08-25 - CPO session projections and customer intelligence deployment

- Deployed enriched CPO charging-session list/detail projections with stable
  customer, charger, hub, and connector fields plus live consumed-kWh overlays
  for active sessions.
- Added the SuperAdmin CPO customer-intelligence API and reconciled its
  OpenAPI contract; no database migration was required.

Verification: focused OpenAPI route parity, full Go tests, vet, diff checks,
local/public readiness, Swagger, OpenAPI, Caddy validation, and the
post-rehost journal scan passed. `pwsh` is unavailable for `verify-docs.ps1`;
disposable PostgreSQL and physical HAL acceptance remain unrun.

## 2026-08-25 - Optional charger SoC telemetry

- Added nullable session SoC persistence and migration 48. CMS accepts the
  additive HAL `transaction.soc` fact with its own ordering sequence, retaining
  initial evidence and rejecting invalid/out-of-order projection updates.
- Customer detail/history, CPO session records, liveops, OpenAPI, and scoped
  operational invalidation now expose actual optional SoC. SoC freshness is
  independent from energy-meter freshness; missing telemetry remains unknown.

Verification: focused CMS packages, full Go tests, vet, OpenAPI route parity,
diff checks, migration 48 application, local/public readiness, Swagger,
OpenAPI, Caddy validation, and the post-rehost journal scan passed. Disposable
PostgreSQL lifecycle and HAL/cpconsole acceptance remain unrun; `pwsh` is not
available for `verify-docs.ps1` on this host.

## 2026-08-21 - Real-hardware CMS/HAL correlation and start-admission hardening

- HAL mutations now reject missing, malformed, zero, uppercase, or otherwise
  noncanonical correlation UUIDs before HTTP. Reconciliation uses a fresh UUID
  for each mapping attempt instead of a textual worker label.
- Added migration 47 and model mapping for bounded mapping diagnostics. CMS
  forwards optional inventory serial evidence to HAL and retains it outside the
  customer API surface.
- Start eligibility now consults fresh online raw OCPP status (`Available` or
  `Preparing`) instead of collapsing that decision into the existing display
  availability projection.

Verification: focused adapter, HAL-operation, CPO, model, live-state, and
charging tests, broad Go package tests, vet, and documentation verification
pass. PostgreSQL migration/lifecycle and physical charger checks remain unrun
without an explicitly disposable database and hardware session.

## 2026-08-21 - CMS/HAL fact-delivery recovery closure

- A repeated customer signup Start now replaces the current CPO/email
  challenge atomically, invalidating and scrubbing its predecessor before the
  successor and its mail-outbox row are committed.
- Added the authenticated platform exact-fact recovery route. It uses the
  server-generated request UUID for HAL correlation, writes audit evidence for
  request/outcome, and emits a platform event only after HAL accepts the same
  immutable fact back to `PENDING`. The source OpenAPI now has 189 operations.

Verification: focused CMS adapter, platform, route/OpenAPI, and customer-auth
tests, full Go tests, vet, and diff checks passed. PostgreSQL
concurrency/lifecycle cases are present but skipped without
`TEST_DATABASE_URL`. Runtime revision `c6b79d4` is active under
`evcmsnew-dev.service` with binary SHA-256
`9a1151f94c34ef9518a3067d9192a28e7c4d9843a2b8564b28399bb9195c8b78`.
Local/public health-readiness, Swagger, raw OpenAPI (189 operations), Caddy
validation, and the post-rehost journal error scan passed. `pwsh` remains
unavailable on this host.

## 2026-08-20 - Auth and current-worker invariant hardening

- Added guarded migration 46 with partial current-challenge uniqueness for
  administrative, customer, and CPO-local signup identities. It fails with a
  diagnostic for pre-existing duplicate current rows rather than silently
  deleting or invalidating them, and it safely requires GST state only after
  proving no existing state is unknown.
- Serialized challenge lifecycle and password authorization on the identity
  owner. Login rechecks credentials and active/lockout state after acquiring
  that lock, so a concurrent password change cannot create an OTP challenge
  from obsolete credentials. Signup serializes its pre-customer identity with a
  transaction-scoped PostgreSQL advisory lock.
- Replaced readiness inference from durable worker-name rows with the current
  process's explicit expected worker-instance set. HAL reconciliation and
  operational retention now report worker health; mail remains required only
  when enabled. Provider-omitted recharge payment methods remain `NULL` in the
  model instead of a synthetic empty value.

Verification: focused auth/customer-auth/platform-ops checks, the complete
`go test -p 1 ./...` suite, `go vet -p 1 ./...`, and `git diff --check` passed.
PostgreSQL concurrency and migration checks require an explicitly selected
disposable `TEST_DATABASE_URL` and were skipped. A verified pre-migration backup
was created and migration 46 was applied; runtime revision `166b0de` is active
under `evcmsnew-dev.service` with binary SHA-256
`07342a2b1a0aa884b12c5f08f24d6721d5946a3db05080b773ed1f882c013b96`.
Local/public health-readiness, Swagger, raw OpenAPI (188 operations), Caddy
validation, and the post-rehost journal error scan passed. `pwsh` remains
unavailable on this host.

## 2026-08-20 - Charging lifecycle occupancy and reconciliation repair

- Added migration 45: an unmaterialized open start intent exclusively reserves
  a connector before materialization; one `ACTIVE`, `STOP_PENDING`, or
  `RECONCILIATION_REQUIRED` session exclusively owns it afterwards. Migration
  guards fail on conflicting existing data and perform no cleanup.
- Added durable session settlement reconciliation, wallet-hold-aware admission,
  STOP command/session convergence, zero-UUID fact rejection, and fail-closed
  HAL transaction lookup validation. Completion evidence is retained even when
  settlement cannot finish safely.
- OpenAPI now exposes `RECONCILIATION_REQUIRED` as a session status.

Verification: focused charging/HAL/CPO/route checks, full
`go test -p 1 ./...`, `go vet -p 1 ./...`, OpenAPI/runtime parity, and
`git diff --check` passed. The new PostgreSQL occupancy test is skipped without
`TEST_DATABASE_URL`. A verified pre-migration backup was created, migration 45
was applied, and runtime revision `765cc80` is active under
`evcmsnew-dev.service` with binary SHA-256
`33a99ca227261065365b00473875f48ade845eb1d72f3a2b21371e2e6dfc412b`.
Local/public health-readiness, Swagger, raw OpenAPI (188 operations), Caddy
validation, and the post-rehost journal error scan passed. `pwsh` remains
unavailable on this host.

## 2026-08-20 - ChargingSession `total_kwh` GORM mapping correction deployed

- Corrected `models.ChargingSession.TotalKWh` to explicitly map to the
  migration-owned `charging_sessions.total_kwh` column. The prior inferred
  `total_k_wh` spelling caused PostgreSQL `42703` during session persistence.
- Audited the other acronym-sensitive CMS model mappings against their existing
  migration columns and expanded the model regression coverage for the
  materially distinct acronym forms. The existing database schema remains
  authoritative and unchanged.

Verification: a PostgreSQL-dialect GORM dry-run now proves the generated
ChargingSession insert contains `total_kwh` and excludes `total_k_wh`; focused
and full Go tests, vet, and `git diff --check` passed. No migration or data
mutation was required. Runtime revision `b11eeed` is active under
`evcmsnew-dev.service` with binary SHA-256
`33a99ca227261065365b00473875f48ade845eb1d72f3a2b21371e2e6dfc412b`.
Local/public health-readiness, Swagger, raw OpenAPI (188 operations), Caddy
validation, and the post-rehost journal error scan passed. `pwsh` and
disposable `TEST_DATABASE_URL` remain unavailable on this host.

## 2026-08-20 - HAL command-response contract hardening

- Made CMS HAL command decoding fail closed: missing command wrappers,
  Go-named/missing fields, zero or mismatched IDs, unsupported kind/state,
  missing timestamps, and zero optional transaction IDs are integration errors
  rather than successful command delivery.
- Applied the nonzero HAL identity invariant to START, STOP, and exact-command
  persistence. Authoritative start evidence repairs a historical pointer-to-zero
  command ID, materializes idempotently, and still rejects a true nonzero
  conflict. No migration or data mutation is required for the source repair.

Verification: `scripts/verify-docs.ps1`, focused
`go test ./src/halclient ./src/halops ./src/customerauth`, full `go test
./...`, `go vet ./...`, and `git diff --check` passed. Disposable PostgreSQL
recovery coverage is present but skipped without `TEST_DATABASE_URL`.

Hosting verification: no migration or data mutation was required. Runtime
revision `1729b41` is active under `evcmsnew-dev.service` with binary SHA-256
`00bcedfc71f42e0ce45cd8df99e1616598d03265ce291fd5e08d9cf8338ddb32`.
Loopback/public health-readiness, Swagger, raw OpenAPI (188 operations), Caddy
validation, zero service restarts, and the post-rehost journal error scan
passed. `pwsh` remains unavailable on this Ubuntu host.

## 2026-08-19 - HAL authoritative-start recovery and fact error classification

- Added transport-neutral HAL projection errors so expected immutable-fact
  validation and correlation failures return stable 4xx/409 responses rather
  than an opaque CMS 500; unexpected persistence/event failures remain
  retryable 500 responses with safe debug classification.
- Added the CMS exact HAL transaction lookup client and bounded stranded-start
  reconciliation. Returned truth uses the same locked materializer as fact
  ingress; a 404 creates no session, while late terminal-intent evidence is
  retained and becomes reconciliation-visible.
- Prevented a delayed synchronous HAL command response from regressing a
  fact-materialized `ACTUALLY_STARTED` intent back to delivery state.

Verification: focused database-free HAL client/config/fact packages passed.
Disposable PostgreSQL state-machine and dual-service virtual-charger acceptance
remain unrun because `TEST_DATABASE_URL` and the topology were not supplied.

## 2026-08-19 - CMS-generated HAL mutation correlation correction

- Corrected both User App charging handlers to pass the canonical
  server-generated `RequestLogger` ID from Gin context into CMS/HAL mapping,
  start, and stop work. The prior code read the incoming `X-Request-ID`
  header, so an ordinary client request supplied an empty correlation value.
- CMS-to-HAL mutation correctness no longer depends on a frontend header.
  The HAL client now rejects an empty or whitespace-only mutation correlation
  locally; non-mutating command lookup remains unchanged. No charging,
  wallet, tariff, GST, OCPP, or reconciliation state-machine semantics changed.

Verification:

- Added handler tests proving CMS-generated correlation reaches both start and
  stop service calls without a client header and does not trust a supplied
  client request ID. Added HAL transport tests for correlation/idempotency
  headers and the no-HTTP empty-correlation guard.
- No migration was required. The source was rebuilt and rehosted as runtime
  revision `d7f72cd`; binary SHA-256 is
  `66a8416d81fc54d397d42c8d4cacb1a866dfa1cc934b5cc6c1ba2b00a46b5754`.
- Post-rehost local/public health/readiness, Swagger, raw OpenAPI (187
  operations), Caddy validation, and the journal error scan passed. `pwsh` and
  disposable `TEST_DATABASE_URL` remain unavailable.

## 2026-08-19 - Charging-start prerequisite and exact-absence recovery correction

- Moved customer-start mapping synchronization ahead of commercial admission;
  `503 charger_mapping_unavailable` now has no start intent, wallet hold, or
  command record. A post-transaction mapping reconfirmation closes an
  inventory-change race and terminalizes its known pre-delivery failure as
  `NOT_ATTEMPTED` with an atomically released hold.
- Reserved `RECONCILIATION_REQUIRED` for an invoked HAL start whose result is
  uncertain. Reconciliation now treats only the typed exact HAL command GET
  404 as confirmed absence: it locks the start command/intent/hold, rejects or
  expires the unmaterialized intent, releases only `HELD`, and records
  `CONFIRMED_ABSENT`. Other HTTP errors, timeouts, unavailable HAL, malformed
  responses, and STOP-command 404 remain conservative reconciliation cases.
- Preserved the plaintext appv1 credential boundary: the credential is hashed
  only in CMS and never replayed. A confirmed-absent command is replaced only
  by a fresh customer start attempt. No migration was needed because the
  existing intent/hold states and unrestricted HAL command record state cover
  the new terminal semantics.

Verification:

- Added a disposable-PostgreSQL focused reconciliation test covering mapping
  prerequisite failure, ambiguous delivery, lookup 500 retention, exact 404
  cleanup/idempotency, fresh retry, exact-found projection, fact race, and
  expired absence. It is skipped locally because `TEST_DATABASE_URL` is unset.
- Database-free focused customerauth/halops tests pass. The source was then
  rebuilt and rehosted as runtime revision `6f65e8e`; binary SHA-256 is
  `e675c0acd9e77ba7e3293f422951f8f4b056881b198327119148738f2536711c`.
- `go test -p 1 ./...`, `go vet ./...`, focused route/OpenAPI parity, and
  `git diff --check` pass. The documentation verifier baseline was reconciled
  from 186 to the authoritative schema's current 187 operations before final
  branch verification. Post-rehost local/public readiness, Swagger, raw
  OpenAPI, Caddy validation, and the journal error scan passed. No migration
  was required. The disposable PostgreSQL reconciliation test remains skipped
  because `TEST_DATABASE_URL` is unset.
## 2026-08-18 - Rehosted CPO wallet transaction reads

- Rebuilt and rehosted revision `2a040e0` with authenticated,
  tenant-scoped `GET /api/v1/cpo/wallet-transactions`. The read supports
  optional `customer_id` filtering and bounded newest-first keyset pagination
  with `limit`, `before`, and `before_id`; no migration was required.
- Binary SHA-256 is
  `1a5dfc0100f85a6159c916880911c5139b5295a7b0a074b42e92668f29a0dc3e`.

Verification:

- Focused CPO/route/OpenAPI tests, full Go tests, and vet passed. The service
  is active with zero restarts on `127.0.0.1:18080`; local/public health,
  readiness, Swagger, raw OpenAPI (187 operations), the unauthenticated route
  boundary, Caddy validation, and the post-rehost error scan passed.

## 2026-08-18 - Deployed temporal tariffs and removed the two requested hubs

- Applied migration 44 after creating the guarded rollback dump
  `/var/backups/postgres/devevcmsnewdb-pre-hub-cleanup-migration-44-20260818-162307.dump`
  (SHA-256 `c64597066e052c83e7f60edc14497727acd9ad89ba628343a462696ae8f9a66e`).
- Removed hubs `63ddfa1f-0c1e-4131-8ad7-eb5835c7cd19` and
  `e00412ec-785e-4c71-85f8-9cb4e30a2d29`, their hub tariffs, and all hub-link
  rows. Retained the five downstream chargers/connectors as unassigned,
  hidden, inactive inventory; no sessions were present.
- Rebuilt and rehosted runtime revision `38625d9`; binary SHA-256 is
  `70626eb4cca88cfcb3a90590b656e197c54bf8c152d198942f756b4146f9101e`.

Verification:

- Migration prechecks and schema/index/trigger checks passed. The service is
  active with zero restarts on `127.0.0.1:18080`; local/public health,
  readiness, Swagger, raw OpenAPI (186 operations), Caddy validation, and the
  post-rehost error scan passed. `TEST_DATABASE_URL` and `pwsh` remain
  unavailable, so disposable-DB lifecycle and PowerShell docs verification
  were not run.

## 2026-08-18 - Hub publication/root tariff concurrency closure

- Amended source-only migration 44 so the customer-visible Hub root-floor
  trigger takes `tariff:<cpo_id>:hub:<hub_id>` before reading the root tariff.
  That is the exact advisory transaction key already used by Hub tariff
  mutations, closing the direct-DB publication/deactivation race.
- Extended static migration coverage to require the Hub guard function, the
  exact lock expression, and lock acquisition before the root-floor query.

Verification:

- Passed the database-free migration static test, repository-wide Go test/vet,
  and documentation verification. PostgreSQL runtime execution was not run.

## 2026-08-18 - Temporal tariff release-blocker closure

- Rejected `customer_visible:true` before initial Hub insertion with the stable
  `409 hub_tariff_root_required` lifecycle: hidden Hub, enabled unbounded Hub
  root tariff, then publish. Hub database-trigger failures now map to the same
  commercial error.
- Amended source-only migration 44 with a null-safe PostgreSQL target
  immutability trigger for tariff CPO/scope/target fields. Price, dates, and
  administrative enablement remain mutable through the existing nested API.
- Preserved fail-closed topology handling while allowing resolver query and GST
  infrastructure errors to reach normal 5xx handling from StartCharging and
  customer price routes.

Verification:

- Database-free focused tests, migration static tests, route/OpenAPI checks,
  full Go test/vet, and docs verification are recorded with this source-only
  slice. PostgreSQL runtime execution is explicitly not performed.

## 2026-08-18 - Temporal tariff fallback source correction

- Added migration 44 and shared commercial temporal policy for immutable Hub,
  Charger, and UserGroup tariff targets: root, start-only open fallback, and
  strictly nested bounded `[start_date,end_date)` overrides. The CMS keeps
  `USERGROUP > CHARGER > HUB` as primary precedence; time never changes
  `is_active` and never configures the separate session-duration cutoff.
- Added CPO-side topology/published-Hub-floor enforcement, scoped tariff
  deletion with historical-reference protection, and the same fail-closed
  resolver for User App informational price and charging admission. Existing
  tariff and Hub-GST snapshots remain immutable.
- Updated canonical OpenAPI, administrative API contract, CPO frontend handoff,
  User App handoff, schema, and network workflow. Added focused temporal and
  migration-static coverage.

Verification:

- Passed focused commercial, CPO, customer-auth, migration-static, and
  route/OpenAPI Go tests. This is not deployed and migration 44 has not been
  applied; disposable PostgreSQL lifecycle/concurrency verification awaits an
  explicitly selected `TEST_DATABASE_URL`.

## 2026-08-18 - Rehosted CPO charger transaction reads

- Rebuilt and rehosted revision `a5d1af4` after adding the tenant-scoped
  `GET /api/v1/cpo/charger-transactions` cursor-paginated read with optional
  charger/customer filters. No migration was required.
- Added the human contract and corrected the OpenAPI financial-status enum to
  include runtime-supported `REFUNDED`; the live contract now contains 183
  operations.
- Binary SHA-256 is
  `c786df05b310487fb84c6a6f85ab4c769249396f32ac6d0f027efb8c0b273669`.

## 2026-08-18 - Rehosted tariff PATCH and frozen GST snapshot hardening

- Made tariff PATCH intent explicit for `units`, `start_date`, and `end_date`:
  omission retains the stored value, explicit `null` clears it, and a concrete
  value replaces it. Hub, Charger, and UserGroup routes now share the same
  application helper and validate the resulting tariff before persistence.
- `units:null` is required when changing to fixed per-session pricing. Both
  schedule dates must be null to clear a schedule; one-sided schedules remain
  invalid. The OpenAPI contract and CPO frontend handoff now state the exact
  request semantics and examples.
- Completion settlement validates its frozen SGST/CGST/IGST snapshot with the
  shared commercial component rule, rejecting missing, out-of-range, or mixed
  split/IGST rates without consulting mutable current Hub/GST state. Released
  legacy tariff-snapshot readers remain unchanged.

Verification:

- Focused database-free CPO and customer-auth tests cover nullable PATCH
  decoding, valid/invalid resulting tariff states, schedule clearing, valid
  GST split/IGST snapshots, and malformed snapshot rejection.
- No migration or runtime database mutation was required. Rebuilt and rehosted
  revision `0ad2de7`; binary SHA-256 is
  `d62b01cad7b25bd4ddd0c82407ce7ee94dd7d03680879edb57057cbd526b9348`.
- Verified focused tariff/GST/route tests, full Go tests, vet, Caddy
  validation, local/public health and readiness, Swagger, raw OpenAPI, auth
  boundaries, and a clean post-start error scan. `pwsh` and disposable
  `TEST_DATABASE_URL` remain unavailable.

## 2026-08-18 - Rehosted CPO analytics and hub charger listing APIs

- Rebuilt and rehosted main revision `47b6e41` after adding tenant-scoped
  `GET /api/v1/cpo/analytics` and
  `GET /api/v1/cpo/hubs/{hub_id}/chargers`; no migration was required.
- Corrected the embedded OpenAPI `AnalyticsResponse` schema to match runtime
  fields and decimal-string serialization. The live contract now has 182
  operations. Binary SHA-256 is
  `a253a3754fdd5240406fb80dd65bb670cede46aa5f74f155fcdb336ad355f01c`.
- Verified focused route/OpenAPI tests, full Go tests, vet, Caddy validation,
  local/public health and readiness, Swagger, raw OpenAPI, new-route auth
  boundaries, and a clean post-start error scan.
- `pwsh` remains unavailable, so `scripts/verify-docs.ps1` was not run.

## 2026-08-18 - Rehosted tariff/GST correction and wallet admission policy

- Applied migrations 42 and 43 after the guarded custom-format rollback dump
  at `/var/backups/postgres/devevcmsnewdb-pre-migration-42-43-20260818-123600.dump`
  (SHA-256 `6123f5c29b9f2c3f826d8b1a87d46905633161c180b75aa86418c0d930f21365`).
- Rebuilt and rehosted runtime revision `ceefb21`; binary SHA-256 is
  `b326c525d0a11a1a1d152f60a08c1906bfa50dc00af0e4db7f080f17b59636e1`.
- Verified migrations 42/43, the `kwh` enum, the CPO/GST uniqueness index,
  settings backfill, zero service restarts, local/public health and readiness,
  Swagger, raw OpenAPI (180 operations), protected worker/status boundaries,
  and a clean post-start error scan.
- `pwsh` and disposable `TEST_DATABASE_URL` remain unavailable, so the
  documentation PowerShell verifier and guarded disposable-DB lifecycle tests
  were not run.

## 2026-08-18 - CPO wallet admission policy in source

- CPO provisioning now creates its one blank settings row transactionally;
  forward migration 43 backfills only CPOs that lack one, preserving all
  existing settings. The two whole-currency wallet policy fields have
  non-negative zero defaults and are now part of the CPO settings API and
  OpenAPI contract.
- New customer charging starts lock the CPO settings with the CPO and wallet,
  require `balance >= wallet_min_balance`, and price the hold/maximum HAL Wh
  from `balance - wallet_buffer_min_balance`. A buffer never changes the
  independent customer/HAL duration cutoff or historical settlement.
- Added CPO frontend guidance and the explicit `409
  wallet_minimum_balance_not_met` admission error; a depleted post-buffer
  balance uses the existing `409 insufficient_wallet_balance` response.
- Customer wallet and wallet-history responses now surface the CPO's current
  minimum/buffer, an exact non-negative usable balance, and the exact amount
  needed to reach the minimum threshold. The recharge amount deliberately does
  not add the buffer.

Verification:

- Database-free CPO, customer-auth, and migration tests cover zero defaults,
  validation, threshold/buffer arithmetic, and resultant affordable Wh limits.
- The guarded PostgreSQL start-admission test additionally covers the locked
  persisted setting, hold, limit, and below-threshold rejection when a
  disposable `TEST_DATABASE_URL` is available.
- This local source change is not deployed and migration 43 has not been run
  against a database.

## 2026-08-18 - Tariff and GST commercial correction in source

- Retained migration 40's data-preserving `price_per_unit` rename and added
  forward migration 42 to rename the persisted `watt/hour` enum value to
  `kwh`, without changing stored numeric prices. New energy tariffs therefore
  mean an exact price per kWh and calculate `meter_wh / 1000 * price_per_unit`.
- Repaired the live tariff/API/model contract to use one supported fixed basis:
  energy per kWh, time per minute, or a fixed per-session amount. Retired
  `watt/hour` is rejected for new writes; released snapshots that used it keep
  their original per-Wh interpretation through an explicit legacy reader.
- Centralized tariff interpretation for customer price display, start
  reservation, immutable tariff snapshots, session materialization, and
  completion settlement. Existing snapshots retain explicit, named legacy
  per-kWh handling only; no new snapshot writes the old key.
- Kept tariff time pricing separate from existing time-bounded-session
  provisioning: actual HAL session timestamps price time tariffs, while the
  current duration cutoff remains unchanged. Hub/GST state and tax components
  now validate as a complete relationship on assignment, replacement, GST
  update, and Hub state update; customer pricing treats invalid relationships
  as unavailable. Active non-zero idle fees are rejected and never exposed as
  a customer billable price.

Verification:

- Focused database-free `db`, `commercial`, `cpo`, and `customerauth` tests
  cover migration text, GST state/rate compatibility, retired-unit rejection,
  all three tariff bases, wallet holds, frozen snapshots, and legacy snapshot
  compatibility.
- Documentation verification, OpenAPI/runtime-route parity, serial
  `go test -p 1 ./...`, and `go vet ./...` passed locally.
- This local source change is not deployed. Migration 42 and the guarded
  PostgreSQL admission/session lifecycle tests have not run because no
  disposable `TEST_DATABASE_URL` is configured.

## 2026-08-18 - Rehosted wallet-balance aggregate and settings migration

- Applied migration 41, adding non-null `wallet_min_balance` and
  `wallet_buffer_min_balance` settings columns with zero defaults.
- Fixed CPO customer aggregate loading so every CPO customer receives wallet
  balance and zero-valued usage/session aggregates even without sessions.
- Rehosted clean runtime revision `040b9bb` with binary SHA-256
  `2b4a0ab8fd77b79ec92bf03a418acaa52eb6320e0e90401c62afbb9d23fced29`.
- Applied migration 41 after a mode-0600 rollback dump at
  `/var/backups/postgres/devevcmsnewdb-pre-migration-41-20260818-110956.dump`.

Verification:

- Focused migration/customer/route tests, serial `go test ./...`, `go vet
  ./...`, Caddy validation, migration/schema checks, local/public
  health/readiness, Swagger, raw OpenAPI, and protected worker/status
  boundaries passed. The service is active with zero restarts and the live
  contract remains at 180 operations.
- `scripts/verify-docs.ps1` and disposable PostgreSQL lifecycle tests remain
  unavailable because `pwsh` and `TEST_DATABASE_URL` are not configured.

## 2026-08-18 - Rehosted tariff unit-price semantic correction

- Migration 40 was applied after a mode-0600 custom-format rollback dump at
  `/var/backups/postgres/devevcmsnewdb-pre-migration-40-20260818-105424.dump`.
  Revision `9e7af67` is active with its then-current watt/hour source contract.

## 2026-08-18 - Rehosted customer aggregates and tariff validation release

- Deployed the latest main release with tenant-scoped customer aggregate
  fields (`total_usage_kwh`, `session_count`, and `wallet_balance`) on CPO
  customer list/detail responses.
- Added request-boundary validation for tariff type, price type, and units so
  unsupported values such as `tariff_type: "Standard"` are rejected as
  validation errors instead of reaching PostgreSQL and becoming a 500.
- Removed four unimplemented tariff/GST DELETE operations from OpenAPI so the
  canonical contract matches the runtime route set. No database migration was
  required; migration 39 remains the latest applied migration.
- Rehosted clean runtime revision `d475b41` with binary SHA-256
  `1ff0938cdbc3fda7b181ac7f788daae8620ecdffc00f060cc227b5e73bae24ba`.

Verification:

- Focused tariff-validation and route/OpenAPI parity tests, serial
  `go test ./...`, `go vet ./...`, Caddy validation, migration-up no-op,
  local/public health/readiness, Swagger, raw OpenAPI, and protected worker
  status boundaries passed. The service is active with zero restarts.
- `scripts/verify-docs.ps1` was not run because `pwsh` is unavailable on this
  Ubuntu host. Disposable PostgreSQL lifecycle and full CMS-to-HAL topology
  acceptance remain pending their dedicated environments.

## 2026-08-14 - Rehosted worker current-instance status projection

- Added migration 39 and current-instance service semantics for the
  single-instance-per-logical-worker deployment. Historical process rows are
  retained, but worker status APIs and SuperAdmin projections return only one
  authoritative current incarnation per worker name.
- A replacement atomically supersedes the old row; later heartbeats from the
  old instance cannot reclaim current authority. Staleness and readiness are
  evaluated from the current row only.
- Added static migration coverage and guarded PostgreSQL restart/replacement,
  stale, and delayed-heartbeat regression coverage.
- Applied migration 39 after a mode-0600 rollback dump at
  `/var/backups/postgres/devevcmsnewdb-pre-migration-39-20260814-141424.dump`.
  Clean runtime revision `11c4c23` was rehosted with binary SHA-256
  `2aa8f6c5cd8e0053a72e36d400c2da87ec84e21e6246544ad4c4082813db7511`.

Verification:

- Migration ledger, current-worker uniqueness, Caddy validation, focused
  migration/platform/routes tests, serial `go test ./...`, serial `go vet
  ./...`, and `git diff --check` passed. The enabled service is active with
  zero restarts; local/public health, readiness, Swagger, raw OpenAPI, and the
  unauthenticated worker/status boundaries passed. The live contract remains
  at 180 operations.
- `scripts/verify-docs.ps1` was not run because `pwsh` is unavailable on this
  Ubuntu host. Disposable PostgreSQL lifecycle and full CMS-to-HAL topology
  acceptance remain pending their dedicated environments.

## 2026-08-14 - Rehosted CPO charger live projection release

- CPO charger list/detail responses now optionally include the committed
  `live` projection with charger connection state and connector
  availability/freshness. Missing projection data remains non-fatal and never
  triggers a synchronous HAL call.
- The list path uses the bounded `liveops.GetChargerDetails` batch reader so a
  page does not create one projection query set per charger. OpenAPI and the
  human administrative contract now describe the optional `live` field.
- No migration was required; migration 38 remains the latest applied
  migration. Clean runtime revision `7350887` was rehosted with binary SHA-256
  `4e5b771835dce699c8198915225654b0e9979c6d38bf603373cbc2b6591c13ab`.

Verification:

- Caddy validation, focused CPO/liveops/customer tests, route/OpenAPI parity,
  serial `go test ./...`, serial `go vet ./...`, and `git diff --check` passed.
  The enabled service is active with zero restarts; local/public health,
  readiness, Swagger, raw OpenAPI, and the protected CPO-route boundary passed.
  The live contract contains 180 operations and the live projection schema is
  present.
- `scripts/verify-docs.ps1` was not run because `pwsh` is unavailable on this
  Ubuntu host. Disposable PostgreSQL lifecycle and full CMS-to-HAL topology
  acceptance remain pending their dedicated environments.

## 2026-08-14 - HAL connection-liveness freshness separation

- Added `HAL_V1_CONNECTION_STALE_AFTER` with a backward-compatible `15m`
  default. Connection freshness is now independent from the existing `30s`
  meter freshness horizon.
- `liveops` now derives connector freshness from an `ONLINE`/fresh parent
  connection rather than meter-age-style expiry. Historical connector status is
  still unavailable/stale when parent connection evidence is stale, offline, or
  unknown; fresh connection evidence never fabricates fresh meter data.
- Recorded the durable Heartbeat fact/sequence contract and a post-deployment
  `bd9099` acceptance procedure. REST/read-side recovery and realtime
  invalidation both consume the same durable HAL fact projection.
- Added focused liveops/config tests and a guarded PostgreSQL connection-fact
  sequence regression that rejects stale observations and duplicate facts.

Compatibility: no public response schema or migration changed. Existing HAL
fact digest validation, receipt idempotency, and connection-sequence ordering
remain authoritative.

Verification: `gofmt`; focused config/liveops/customerauth tests; `go test
./...`; `go vet ./...`; and `scripts/verify-docs.ps1` passed with
`TEST_DATABASE_URL` removed. The PostgreSQL projection regression skipped
safely because no disposable database was selected.

## 2026-08-13

### Rehosted CPO charging-session read release

- Added tenant-scoped CPO `ADMIN` list/detail reads for materialized charging
  sessions with bounded `(created_at, id)` pagination, validated lifecycle and
  UUID filters, and no HAL call or cross-CPO access.
- Completed the OpenAPI contract with both operations and the `SessionStatus`
  enum, and updated the administrative API contract plus CPO frontend handoff.
- No database migration was required; migration 38 remains the latest applied
  migration. Clean runtime revision `4cb1edd` was rehosted with binary SHA-256
  `bba0cdc3305c16e339dc4e6e8b7e55624e5fcb835c2c8ea16755d98921f5e91b`.

Verification:

- Caddy validation, focused route/OpenAPI parity, serial `go test ./...`,
  serial `go vet ./...`, and `git diff --check` passed. The enabled service is
  active with zero restarts; local/public health, readiness, Swagger, raw
  OpenAPI, and the unauthenticated new-route boundary passed. The live contract
  contains 180 operations.
- `scripts/verify-docs.ps1` was not run because `pwsh` is unavailable on this
  Ubuntu host. Disposable PostgreSQL lifecycle and full CMS-to-HAL topology
  acceptance remain pending their dedicated environments.

### Rehosted commercial-tax and charger-hub prerequisite release

- Applied migration `000038_separate_tariff_gst_and_require_charger_hub` after
  a validated mode-0600 PostgreSQL rollback dump. The migration removed legacy
  tariff GST ownership only after confirming zero non-null associations,
  normalized hubless chargers to hidden/inactive, and installed durable
  hub-prerequisite checks.
- Built and rehosted clean runtime revision `ebb57fb`, retaining the prior
  binary at `builds/evcmsnew.pre-ebb57fb`.

Verification:

- Caddy validation, route/OpenAPI parity, focused CPO/customer/model tests,
  serial `go test ./...`, serial `go vet ./...`, and `git diff --check` passed.
- Migration ledger/constraint checks, active service state with zero restarts,
  loopback/public liveness and readiness, Swagger, raw OpenAPI, the live
  178-operation contract, and post-rehost warning checks passed. No DNS, Caddy
  route, or HAL provider configuration changed.
- `scripts/verify-docs.ps1` was not run because `pwsh` is unavailable on this
  Ubuntu host. Disposable PostgreSQL lifecycle and full CMS-to-HAL topology
  acceptance remain pending their dedicated test environment.

### Commercial tax and charger hub-prerequisite correction (local, unhosted)

- Separated tariff commercial fields from GST ownership. Migration 38 aborts
  before dropping any non-null legacy tariff GST association; it never infers
  or copies a tax assignment. Hub GST remains the same-CPO tax authority.
- Customer price and new-start snapshots now combine the effective tariff with
  active hub GST. A zero price or zero rate is available configuration, while
  a missing tariff and missing/inactive hub GST remain distinct unavailable
  conditions. Completion uses frozen SGST+CGST+IGST snapshots.
- Added durable and service-layer safeguards that keep hubless chargers hidden
  and inactive. The frozen HAL/OCPP contracts were not changed.
- Focused CPO, customer-auth, model, route/OpenAPI, and documentation checks
  passed locally before the guarded migration and rehost recorded above.

### Hosted tariff-targeting correction and User App publication sweep

- Added migration `000037_correct_tariff_targeting`: legacy composite rows are
  normalized with `usergroup > charger > hub` precedence; every tariff now has
  exactly one nullable target ID and matching non-null `assigned_to`; the
  charger foreign key is CPO-local without requiring a hub. The down migration
  is deliberately guarded against non-representable charger/user-group data.
- Made nested CPO tariff paths the sole target authority. Create/PATCH JSON
  bodies accept commercial fields only, response rows expose `assigned_to`, and
  scoped CRUD cannot read or move a tariff under another target type.
- Replaced competing User App pricing queries with one selector shared by hub
  price, charger price, and charging-start tariff snapshots:
  `USERGROUP > CHARGER > HUB`. It does not depend on SQL row order.
- Completed the charger publication sweep by sharing the existing visible
  charger + visible hub predicate with favorite creation and charger-image
  access. Existing customer-owned sessions remain accessible after unpublish.
- No HAL/OCPP, legacy CMS, QR, wallet settlement, or session ownership contract
  changed.

Verification:

- `go test ./...`, `go vet ./...`, focused `db`, `cpo`, `customerauth`, models,
  and route/OpenAPI parity tests passed. The migration static test covers
  normalization, constraints, same-CPO charger ownership, and guarded rollback.
- CPO lifecycle regressions and User App precedence regressions are guarded by
  `TEST_DATABASE_URL`; they are skipped because no disposable database is
  selected. `pwsh` documentation verification is unavailable on this host.
- The mode-0600 pre-migration dump is
  `/tmp/devevcmsnewdb-pre-a9fc32b.dump` with SHA-256
  `1ecec3a7e73b3417fa303239d0938af3f9991a40c1b016317724aea909515e1a`.
  Migration 37 applied with zero tariff rows to normalize. Runtime revision
  `a9fc32b` was rehosted with binary SHA-256
  `0b8c57d7991511e55d9d9200f57961b692ff5acd330968cd9345bbeb517884a1` and
  `vcs.modified=false`; service, Caddy, database constraints, loopback/public
  health and readiness, Swagger, raw OpenAPI, 178-operation parity, protected
  route boundaries, and post-rehost journal checks passed.

### Hosted charger customer-visibility release

- Applied additive migration `000036_add_customer_visibility_to_chargers` with
  `customer_visibility BOOLEAN NOT NULL DEFAULT TRUE`.
- Completed the charger visibility contract: CPO ADMIN publication updates,
  OpenAPI/human API documentation, schema mapping, and User App discovery,
  direct lookup, pricing, and favorite enforcement. Existing sessions and
  active charging are not revoked by unpublishing.
- Backed up `devevcmsnewdb` before migration to the mode-0600 dump
  `/tmp/devevcmsnewdb-pre-18f261e.dump` (SHA-256
  `234a98f1ba69e8aeb5fd35353c1ed97486a0e4c89b8a84e9509a7393fe97abbb`) and
  retained the prior binary at `builds/evcmsnew.pre-a9528c4`.

Verification:

- Full serial Go tests, serial vet, formatting, focused route/OpenAPI parity,
  and diff checks passed. `pwsh` documentation verification was skipped because
  PowerShell is unavailable; disposable PostgreSQL lifecycle tests remain
  skipped because `TEST_DATABASE_URL` is unavailable.
- Clean runtime revision `a9528c4` was rehosted with binary SHA-256
  `09b80b12865866b84e8a690bbaa2829257a036af8b2fee5c69fb3a7808a4b60c` and
  `vcs.modified=false`; service, Caddy, database, loopback/public health and
  readiness, Swagger, raw OpenAPI, 178-operation parity, unauthenticated route
  boundary, and post-rehost journal checks passed.

### Rehosted User App charging-start admission hardening

- New customer charging starts now require committed connector availability to
  be `AVAILABLE` and `FRESH`; unavailable, stale, offline, or faulted states
  return `409 connector_not_available` before charging side effects. Same-
  customer active-intent replay remains idempotent, and connector row locking
  serializes competing starts.
- No migration or HAL/OCPP contract changed.

Verification:

- Focused admission tests, route/OpenAPI parity, serial full Go tests, serial
  vet, formatting, and diff checks passed. The disposable PostgreSQL lifecycle
  remains skipped because `TEST_DATABASE_URL` is unavailable.
- Clean revision `172bcd4` was rehosted with binary SHA-256
  `ab53143ae0bb55d14e9256d77eb5bf3350ce1aed2c280236b41dc4ad80ea2238` and
  `vcs.modified=false`; service state, Caddy, migration/table, health,
  readiness, Swagger, raw OpenAPI, unauthenticated charging-start, and
  post-rehost journal checks passed. The prior binary is retained at
  `builds/evcmsnew.pre-172bcd4`.

### Hardened User App charging-start admission against stale or unavailable live state

- New User App charging starts now require the CMS-owned `liveops` connector
  projection to report `AVAILABLE` and `FRESH` before the wallet hold, durable
  start intent, durable HAL command record, or HAL delivery path can begin.
  The change does not add a route, schema, migration, HAL/OCPP contract, or
  synchronous CMS-to-HAL availability call.
- Same-customer active-intent replay remains available before the live-state
  gate. Another customer receives only the existing generic
  `409 connector_not_available` response. The final transaction locks the
  connector row and rechecks active intent ownership to serialize competing
  starts.
- Updated the canonical OpenAPI and User App frontend handoff: enable start
  only for `AVAILABLE` plus `FRESH`, treat the backend as authoritative, and
  on `409` refetch charger detail rather than auto-retrying. Existing SSE and
  REST recovery semantics are unchanged.

Verification:

- Focused `customerauth` admission tests, route/OpenAPI runtime parity, the
  PowerShell documentation verifier, `go test -p 1 ./...`, `go vet -p 1 ./...`,
  and `git diff --check` passed.
- The added PostgreSQL lifecycle regression is guarded by
  `TEST_DATABASE_URL`; it is skipped locally because no disposable database is
  selected. Physical charger and real CMS-to-HAL-to-charger acceptance were
  not run and remain outside this CMS-only source slice.

### Rehosted state-aware GST-to-hub assignment validation

- Added same-state versus different-state GST rate validation to hub assignment
  and replacement. Same-state hubs require SGST and CGST and reject non-zero
  IGST; different-state hubs require IGST and reject non-zero SGST or CGST.
  Invalid combinations return `400 invalid_gst_for_hub`.
- Updated the OpenAPI and administrative CPO contracts. No migration or HAL
  change was required.

Verification:

- Focused CPO tests, route/OpenAPI parity, serial full Go tests, serial vet,
  formatting, and diff checks passed. The disposable PostgreSQL lifecycle
  remains skipped because `TEST_DATABASE_URL` is unavailable.
- Clean merge revision `4377383` was rehosted with binary SHA-256
  `769b71782f47bb93c37e063dbab4ba8af34902b80ac8fb3150c21ac61c2fc5e0` and
  `vcs.modified=false`; service state, Caddy, migration/table, health,
  readiness, Swagger, raw OpenAPI, GST route boundaries, and post-rehost
  journal checks passed. The prior binary is retained at
  `builds/evcmsnew.pre-4377383`.

### Completed and rehosted the CMS-side User App charging history release

- Added customer/CPO-scoped materialized charging-session history with bounded
  keyset pagination, safe card projections, and additive historical detail
  fields backed by persisted session, frozen tariff/tax, payment, and wallet
  relations. Unfinished sessions do not present provisional zero totals as a
  final bill.
- Corrected the operational-event correlation bug: transaction events now use
  the materialized CMS charging-session UUID for `CHARGING_SESSION.resource_id`,
  never the start-intent UUID. Stale meter sequences remain suppressed.
- Updated OpenAPI (177 operations), the User App handoff, operational event
  contract, project state, plan, and documentation verifier. No migration or HAL
  change was required; physical hardware acceptance remains deferred.

Verification:

- Focused customerauth/liveops/operationalrealtime/models checks, route/OpenAPI
  runtime parity, the PowerShell documentation verifier, `go test -p 1 ./...`,
  `go vet -p 1 ./...`, and `git diff --check` passed.
- `TEST_DATABASE_URL` is not configured, so disposable PostgreSQL lifecycle
  verification is skipped rather than passed. Physical charger and real
  CMS-to-HAL-to-charger acceptance remain deliberately deferred.
- Clean revision `87b8727` was then rehosted. The installed binary SHA-256 is
  `007d40cbd9eeda79392f7b1d546cc4d6e2bf336913212c6ce9f17dde4f9a6434`, with
  `vcs.modified=false`; service state is active/enabled with zero restarts.
  Caddy, migration/table, loopback/public health/readiness, Swagger, raw
  OpenAPI, 177-operation, unauthenticated history-route, and post-rehost
  journal checks passed. The prior binary is retained at
  `builds/evcmsnew.pre-87b8727`.

### Rehosted clean HAL runtime model mapping release

- Corrected GORM table ownership for `HALChargerRuntime` and
  `HALConnectorRuntime` so they map to the singular migration tables
  `hal_charger_runtime` and `hal_connector_runtime`. Added focused model
  regression coverage and formatted the affected release sources.
- Published the formatter correction as `0d50c09` and rehosted a clean build.
  No database migration was required; the live ledger remains through
  migration thirty-five. The prior active binary is retained at
  `builds/evcmsnew.pre-0d50c09`.

Verification:

- Model regression, route/OpenAPI parity, `go test -p 1 ./...`,
  `go vet -p 1 ./...`, clean build, and `git diff --check` passed.
- The clean installed binary SHA-256 is
  `e3790854e68f7a3996d50a552e2f15ef6a95f644184e10389a95d513d64b24bf`, with
  `vcs.modified=false`. Service state is active/enabled with zero restarts;
  Caddy validation, migration/table checks, loopback/public health and
  readiness, Swagger, raw OpenAPI, the 176-operation contract, and the
  post-rehost journal scan passed.
- `scripts/verify-docs.ps1` remains unrun because `pwsh` is unavailable on
  this Ubuntu host. Disposable PostgreSQL lifecycle and full HAL topology
  acceptance remain pending because `TEST_DATABASE_URL` and the provider test
  environment are not configured.

## 2026-08-12

### Rehosted GST-to-hub assignment release

- Fast-forwarded `main` through the GST hub-assignment release, applied
  migration `000035_add_gst_id_to_hubs` after a validated mode-0600 PostgreSQL
  rollback dump, and deployed corrected revision `e831b32` after repairing the
  GST hub OpenAPI indentation/reference drift found by route-contract tests.
- The live service now exposes four GST-hub operations, including assign,
  retrieve, replace, and unassign, with the `hubs.gst_id` same-CPO foreign-key
  boundary. No DNS, Caddy route, or HAL provider configuration changed.

Verification:

- Caddy validation, OpenAPI/runtime parity, serial full Go tests, vet, clean
  build, and `git diff --check` passed.
- Migration ledger/column/constraint checks, active service state with zero
  restarts, local/public liveness and readiness, Swagger, raw OpenAPI, the
  176-operation contract, and unauthenticated 401 checks for all four GST-hub
  routes passed.
- `scripts/verify-docs.ps1` was not run because `pwsh` is unavailable on this
  Ubuntu host. Disposable PostgreSQL lifecycle and full CMS-to-HAL topology
  acceptance remain pending their dedicated test environment.

### Expanded the canonical CPO HAL/live-charger integration guide

- Reworked the existing CPO HAL operational-capability manual into a
  source-grounded junior reference covering ownership, public capability
  signatures/returns, identifiers, static versus live derivation, facts,
  mapping/reconciliation, charging lifecycle, REST/SSE recovery, testing, and
  explicit red-zone boundaries.
- Made the CPO handoff reading path and documentation map point directly to the
  canonical guide. No runtime, migration, OpenAPI, or HAL provider behavior was
  changed.

Verification:

- The PowerShell documentation verifier and `git diff --check` passed. The
  manual's cited CMS symbols, current routes, migrations, configuration, and
  read-only provider contract were cross-checked before editing.

### Reconciled CPO charger and live-operations integration

- Removed accidental duplicate CPO operational response declarations from the
  service layer while retaining schema ownership and all liveops-backed CPO
  reads. Charger creation now performs one post-commit normal mapping push
  (`cpo-charger-created`) rather than an additional update-labelled push.
- New charger creation uses its one generated six-character public
  `charger_id` as `ocpp_identity`; connection URLs preserve the stored identity
  without prefix rewriting. Existing rows are not rewritten.
- Corrected the migration-34 model contract to nullable
  `tariffs.assigned_to tariff_assignment_type`, matching its additive migration
  and current unassigned tariff behavior.

Verification:

- `go test ./src/cpo -count=1`, `go test ./src/liveops -count=1`,
  `go test ./src/customerauth -count=1`, `go test ./src/halops -count=1`,
  route/OpenAPI parity, and the PowerShell documentation verifier passed.
  `go test -p 1 ./...`, `go vet -p 1 ./...`, and `git diff --check` passed;
  serial Go verification was required because the ordinary parallel full suite
  exhausted the Windows paging file while starting test processes.
- No migration, deployment, HAL checkout modification, or forced history
  operation is part of this repair. `TEST_DATABASE_URL` is not configured, so
  disposable migration/lifecycle verification was not run.

### Rehosted the reconciled CPO and tariff-assignment release

- Fast-forwarded `main` to revision `2e8fdb3`, applied migration
  `000034_add_assigned_to_to_tariffs` after a validated mode-0600 PostgreSQL
  rollback dump, and retained the prior binary at
  `builds/evcmsnew.pre-2e8fdb3`.
- The live service now includes the reconciled CPO/HAL mapping behavior,
  charger identity compatibility, and nullable tariff assignment metadata.
  No DNS, Caddy route, or HAL provider configuration changed.

Verification:

- Caddy validation, route/OpenAPI parity, focused CPO/model tests,
  `go test ./...`, `go vet ./...`, clean build, and `git diff --check` passed.
- Migration ledger/type/column checks, active service state with zero restarts,
  loopback/public liveness and readiness, Swagger, raw OpenAPI, the live
  172-operation contract, and post-rehost warning scan passed.
- `scripts/verify-docs.ps1` was not run because `pwsh` is unavailable on this
  Ubuntu host. Disposable PostgreSQL lifecycle and full CMS-to-HAL topology
  acceptance remain pending their dedicated test environment.

### Deployed User App live-state consumption and CPO HAL manual

- Replaced the single-detail-only customer availability overlay with a shared
  bounded `liveops.GetChargerDetails` projection read used by full charger
  lists, hub detail, favorites, and single detail. Compact map locations remain
  unchanged and expose only name/coordinates. Static CMS charger/connector
  lifecycle is kept separate from HAL evidence and safely overrides displayed
  availability when not active.
- Added `integrations/cpo-hal-operational-capability-manual.md`, the canonical
  CPO backend guide for ownership, package sockets, fact fields/identity,
  mapping/command/fact lifecycles, reads/SSE/recovery, future command design,
  configuration, disposable topology, and anti-patterns. Corrected stale User
  App, CPO handoff, docs-map, and OpenAPI descriptions.

- Built and rehosted clean `main` revision `27c01f3` without a migration or
  HAL checkout change, retaining the prior binary at
  `builds/evcmsnew.pre-27c01f3`.

Verification:

- Caddy validation, focused live-projection and route/OpenAPI tests,
  `go test ./...`, `go vet ./...`, clean build, and `git diff --check` passed.
- Active/enabled systemd state with zero restarts, loopback/public liveness and
  readiness, Swagger, raw OpenAPI, the unchanged 172-operation contract, and
  the deployed User App availability descriptions passed. No post-restart
  warnings were recorded.
- `scripts/verify-docs.ps1` was not run on this Ubuntu host because `pwsh` is
  unavailable. Disposable dual-service lifecycle evidence remains pending.

### CMS HAL operational capability layer started

- Separated low-level HAL wire transport from CMS operation, shared fact-ingress,
  and projection-read capabilities; added CPO/Platform live snapshots, User
  App safe availability/session projections, durable operational events, and
  scoped cursor/SSE recovery for CPO ADMIN, Platform-per-CPO observation, and
  CPO-local customers.
- Stale connection, connector, and meter sequences do not publish new event
  records. Inventory mapping is marked pending before post-commit HAL delivery
  and resynchronizes after charger edits/status changes.
- Compile and OpenAPI route-parity checks passed. No migration, deployment,
  commit, push, or provider change occurred; PostgreSQL lifecycle, SSE behavior,
  reconciliation recovery, and dual-service topology verification remain pending.

### Rehosted the CMS HAL operational capability release

- Built and rehosted clean `main` revision `3f3a952` on the development VPS.
  Applied migration `000033_operational_realtime_events` after a validated
  mode-0600 PostgreSQL rollback dump and retained the prior binary at
  `builds/evcmsnew.pre-3f3a952`.
- The live CMS now has the durable, CPO/customer-scoped `operational_events`
  ledger (four indexes), the 172-operation OpenAPI surface, and the HAL fact
  ingress validation boundary at `POST /v1/hal-facts`. No DNS, Caddy route, or
  HAL provider configuration changed.

Verification:

- Caddy validation, route/OpenAPI parity, `go test ./...`, `go vet ./...`,
  clean build, and `git diff --check` passed.
- Migration ledger/table/index checks, active/enabled systemd state with zero
  restarts, loopback/public liveness and readiness, Swagger, raw OpenAPI, and
  HAL fact-ingress validation smoke checks passed.
- `scripts/verify-docs.ps1` was not run because `pwsh` is unavailable on this
  Ubuntu host. The disposable PostgreSQL lifecycle suite and full CMS-to-HAL
  topology acceptance remain pending because their dedicated test environment
  is not configured.

## 2026-08-11

### Rehosted latest main revision and reconciled deployment records

- Built and rehosted clean `main` revision `3ca2c35` on the development VPS.
  Applied migration `000032_add_state_to_hubs`, preserving the prior binary at
  `builds/evcmsnew.pre-3ca2c35` and a validated mode-0600 rollback dump at
  `/tmp/devevcmsnewdb-pre-3ca2c35.dump`.
- The live service now exposes the hub `state` contract and authenticated User
  App charger locations, with 162 OpenAPI operations. No DNS, Caddy route, or
  HAL provider configuration changed.

Verification:

- Focused route/OpenAPI, CPO, and User App tests, `go test -p 1 ./...`,
  `go vet -p 1 ./...`, clean build, and `git diff --check` passed.
- Migration ledger/table checks, active/enabled service state with zero
  restarts, loopback/public liveness and readiness, Swagger, raw OpenAPI,
  protected-route `401`, and post-rehost journal checks passed. The old process
  emitted a cached-plan result-type warning immediately after the schema change;
  the rehost replaced it and the new process has no error, panic, or fatal
  records.
- `scripts/verify-docs.ps1` was not run because `pwsh` is unavailable on this
  Ubuntu host. The disposable PostgreSQL lifecycle suite remains skipped because
  `TEST_DATABASE_URL` is not configured; the live migration check above used the
  explicitly authorized development database.

### Reconciled CPO hub state response contract

- Completed the merged hub-state slice by adding `state` to CPO hub list/detail
  response DTOs and the matching OpenAPI, human API, and CPO handoff contracts.
  Create requires a non-empty state; update accepts a non-empty replacement.
- This is a source and contract repair only. No migration or deployment was
  performed during branch reconciliation.

Verification:

- `go test -p 1 ./src/cpo ./src/customerauth -count=1`,
  `./scripts/verify-docs.ps1`, and `git diff --check` passed after the merged
  source repair. Broader route/full-suite verification remains constrained by
  the workstation's Go runtime memory limit.

### Added compact User App charger-location discovery

- Added authenticated `GET /api/v1/app/chargers/locations`, reusing the full
  published-charger filter, pagination, and near-me semantics while returning
  each map marker as only `charger_name`, attached-hub `latitude`, and
  attached-hub `longitude`.
- Kept CPO isolation and the customer-visible hub boundary intact. No database
  migration, deployment, live availability claim, or HAL interaction was
  added.

Verification:

- Focused customer-auth tests pass, including an exact-three-fields compact
  projection serialization test. PowerShell documentation-contract verification
  passes with the expected 158 OpenAPI operations.
- The route/OpenAPI Go test remains unverified on this workstation: one normal
  run exhausted the Go runtime's available memory, and two single-process,
  capped-memory retries exceeded the two-minute bound while compiling. No
  route test failure was reported.
### Rehosted authenticated CPO invoice-logo retrieval

- Added `GET /api/v1/cpo/settings/invoice-logo`, which streams the current
  tenant's uploaded invoice logo with inline, byte-range, and no-sniff headers.
  The route derives CPO scope from the authenticated ADMIN session and never
  accepts a filesystem path from the caller.
- Corrected the OpenAPI path nesting so Swagger and the raw schema expose the
  route as a standalone operation. No migration was required.
- Built clean source revision `d368903` and rehosted the development CMS. The
  live OpenAPI now exposes 161 operations.

Verification:

- Idempotent migration-up execution, OpenAPI/runtime route verification,
  `go test ./...`, `go vet ./...`, and `git diff --check` passed. The deployed
  binary SHA-256 is `bf9c5bdd7ac9205b45086a513b014bce1c371991a4a61ab23ff9541ee5f9eb07`
  and embeds `d368903a960c519293b8ea26a9415902eb381056` with
  `vcs.modified=false`.
- The enabled service is active with zero restarts. Local/public liveness and
  readiness, Swagger, raw OpenAPI, and unauthenticated settings, invoice-logo,
  GST, and SuperAdmin route checks passed.
- The bounded SSE shutdown deadline appeared during rehost as expected;
  systemd recovered the process and the current result is `success`.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

### Rehosted GST state support and migration 31

- Added and applied `000031_add_state_to_gsts`, adding an optional stored state
  column and allowing legacy GST-rate columns to remain nullable.
- Added required GST-state validation on create and supplied-state validation on
  update, matching the OpenAPI contract. Create requests still require all
  three GST rates; nullable rates are only possible for legacy/database rows.
- Built clean source revision `a76d6ae` and rehosted the development CMS. The
  live OpenAPI remains at 160 operations.

Verification:

- Migration-up execution, OpenAPI/runtime route verification, `go test ./...`,
  `go vet ./...`, and `git diff --check` passed. The deployed binary SHA-256 is
  `0a3d397464dae13ef15b090225b4ca38fb1b4dfff946bf0de7d77cb9a5d3ebc0` and
  embeds `a76d6ae09dde7727661238f55e1b2ff5007394d0` with `vcs.modified=false`.
- The enabled service is active with zero restarts. Local/public liveness and
  readiness, Swagger, raw OpenAPI, and unauthenticated GST, settings, and
  SuperAdmin route checks passed.
- The bounded SSE shutdown deadline appeared during rehost as expected;
  systemd recovered the process and the current result is `success`.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

### Rehosted CPO settings API and migration 30

- Added and applied `000030_add_settings_table` for one tenant-scoped settings
  row per CPO, including invoice note and logo metadata.
- Corrected the migration to use the repository's `gen_random_uuid()` and
  `cpos` conventions, and made the settings OpenAPI request/response schemas
  match the actual multipart handler. POST and PUT now have distinct operation
  IDs.
- Built clean source revision `e5fd599` and rehosted the development CMS. The
  live OpenAPI now exposes 160 operations.

Verification:

- Migration-up execution, OpenAPI/runtime route verification, `go test ./...`,
  `go vet ./...`, and `git diff --check` passed. The deployed binary SHA-256 is
  `b6c251b9a65343db5eedbff3fee293678f4ac51cf401975b1d77380b1e47ef84` and
  embeds `e5fd599790b4fd9983ba055fc03b10637c2ad674` with `vcs.modified=false`.
- The enabled service is active with zero restarts. Local/public liveness and
  readiness, Swagger, raw OpenAPI, and unauthenticated settings and SuperAdmin
  route checks passed.
- The bounded SSE shutdown deadline appeared during rehost as expected;
  systemd recovered the process and the current result is `success`.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

### Rehosted nullable tariff metadata and SuperAdmin fixes

- Added migration `000029_add_tariff_fields` for nullable tariff metadata
  (`tariff_type`, `price_type`, and `units`), preserving nulls for existing
  tariffs and omitted request fields.
- Corrected the SuperAdmin administrator-list model binding and repaired the
  OpenAPI `UserGroup.members` contract, including removal of a duplicate schema
  key. The live contract remains at 157 operations.
- Built clean source revision `2550cf7` and rehosted `evcmsnew-dev.service`;
  migration 29 was already applied and the service is enabled and active.

Verification:

- OpenAPI/runtime route verification, `go test ./...`, `go vet ./...`,
  migration-up execution, and `git diff --check` passed. The binary embeds
  revision `2550cf79fa6a9b84f3e30b0dca4101b8f0659574` with `vcs.modified=false`.
- Loopback/public liveness and readiness, Swagger, raw OpenAPI, the live
  tariff/member fields, and protected-route `401` checks passed. The live
  OpenAPI exposes 157 operations.
- During the bounded stop phase, the known long-lived SSE connection reached
  the graceful-shutdown deadline (`context deadline exceeded`); systemd
  restarted the process successfully. The current service reports
  `Result=success`, `NRestarts=0`, and is healthy.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

### Added the initial CMS HAL v1 charging consumer slice

- Added migration `000028_cms_hal_charging_vertical` and CMS-side start intent,
  wallet hold, command, immutable fact receipt, mapping, and operational
  projection records.
- Added the HAL v1 HTTP client, separate HAL fact receiver authentication,
  RFC 8785 JCS fact-digest verification, customer charging start/stop/polling
  routes, and matching OpenAPI/configuration/integration documentation.
- Kept the HAL provider, legacy HAL, and legacy CMS untouched. No deployment,
  provider credentials, or virtual-charger acceptance was assumed.
- Applied migration `000028_cms_hal_charging_vertical`, built clean source
  revision `f7e7227`, and rehosted the development CMS. The live contract now
  exposes 157 operations.

Verification:

- Focused CMS package, migration/model, and OpenAPI/runtime route tests passed.
- `go test ./...` and `go vet ./...` passed before activation. The enabled
  systemd service is active on `127.0.0.1:18080`; local/public liveness and
  readiness, Swagger, raw OpenAPI, migration 28, and the new protected charging
  routes passed live checks. The deployed binary embeds `f7e7227` with
  `vcs.modified=false` and the post-start warning journal was empty.
- The current ignored environment intentionally leaves `HAL_V1_BASE_URL` and
  both HAL credentials unset; the charging start route therefore remains
  unavailable until the independent v1 provider is configured.
- The full dual-PostgreSQL HAL plus virtual charger vertical, restart/outage
  cases, and reconciliation worker are not yet verified and must not be
  presented as complete acceptance.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

## 2026-08-10

### Rehosted CPO user-group detail members

- Deployed the CPO user-group detail response with its safe `members` customer
  projections, preserving the tenant-scoped membership boundary.
- Completed the OpenAPI contract for `UserGroup.members` and the CPO customer
  `usergroup_assigned` field, then aligned the human administrative API contract.
- No database migration was introduced; the live database remains at migration
  twenty-seven.
- Built and rehosted clean source revision `6930189`; the live contract remains
  at 152 operations.

Verification:

- Migration-up check completed with no pending migration; OpenAPI/runtime route
  verification, `go test ./...`, and `go vet ./...` passed.
- The deployed binary embeds revision `6930189` with `vcs.modified=false`;
  systemd is enabled and active on `127.0.0.1:18080`.
- Local/public liveness and readiness, Swagger UI, live OpenAPI, the new
  `UserGroup.members` and `usergroup_assigned` fields, protected route `401`
  responses, and the post-start warning scan passed.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

### Rehosted CPO user-group membership completion

- Completed the CPO user-group membership contract with the tenant-scoped
  `DELETE /api/v1/cpo/user-groups/{user_group_id}/members/{customer_id}` route
  and the `usergroup_assigned` customer projection field.
- Reconciled the OpenAPI operation ID, tag, canonical error-envelope references,
  and customer response schema, then aligned the human administrative API
  contract with idempotency, tenant validation, conflict, and audit semantics.
- No database migration was introduced; the live database remains at migration
  twenty-seven.
- Built and rehosted clean source revision `8cb1317`; the live contract now
  exposes 152 operations.

Verification:

- OpenAPI/runtime route-contract verification, `go test ./...`, and `go vet ./...`
  passed.
- The live delete operation and `usergroup_assigned` schema field are present;
  the protected user-group listing route rejects unauthenticated requests.
- Local/public liveness and readiness, Swagger UI, and the live OpenAPI passed.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

### Clarified Superadmin Swagger group headings

- Renamed all platform-management OpenAPI tags to use an explicit `Superadmin -`
  prefix: Platform CPOs, Platform Operations, Governance, and Manual
  Subscriptions.
- Updated every operation assignment so Swagger groups all Superadmin-side APIs
  under clearly demarcated headings; endpoint paths and behavior are unchanged.

Verification:

- OpenAPI/runtime route-contract verification passed.
- `git diff --check` passed.

### Rehosted CPO user-group member assignment

- Added the tenant-scoped `POST /api/v1/cpo/user-groups/{user_group_id}/members`
  operation, with CPO authorization, same-CPO customer validation, idempotent
  same-group assignment, conflict handling for another group, audit evidence,
  and matching OpenAPI coverage.
- No database migration was introduced; the live database remains at migration
  twenty-seven.
- Built and rehosted source revision `f88084d`; the live contract now exposes
  151 operations.

Verification:

- OpenAPI/runtime route-contract verification, `go test ./...`, and `go vet ./...`
  passed.
- The member-assignment route is present and protected; unauthenticated access
  returns `401`.
- Local/public liveness and readiness, Swagger UI, and the live OpenAPI passed.
- A connected platform realtime SSE client caused the old process to hit its
  bounded graceful-shutdown deadline during restart; systemd automatically
  restarted the service, which is active and healthy. No persistent startup
  fault was observed.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

### Reconciled scoped CPO tariff and user-group contracts

- Added and reconciled tenant-scoped CPO tariff operations for hubs, chargers,
  and user groups, plus CPO user-group CRUD, with matching service validation,
  authorization, OpenAPI, and route-contract coverage.
- No database migration was introduced; the live database remains at migration
  twenty-seven.
- Built and rehosted source revision `1b298a7`; the live contract now exposes
  150 operations.

Verification:

- OpenAPI/runtime route-contract verification, `go test ./...`, and `go vet ./...`
  passed.
- Protected tariff and user-group routes return `401` without credentials.
- Local/public liveness and readiness, Swagger UI, live OpenAPI, and the
  post-restart warning journal passed.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

### Propagated safe CPO charger metadata to User App discovery

- Added `charger_name`, `charger_type`, `segment`, `sub_segment`,
  `charger_use_type`, and `parking` to the authenticated User App charger
  projection used by discovery, detail, hubs, and favorites.
- Preserved the customer boundary: charger-host contact details, connection
  URLs, sanctioned load, and HAL-owned live state are not exposed.
- Reconciled CPO connector create/update documentation and the embedded OpenAPI
  example with the current runtime request field
  `connector_total_capacity`, eliminating the contract-validation failure.
- Built and rehosted source revision `13479fe` on the development VPS without a
  new migration; the live database remains at migration twenty-seven.

Verification:

- `go test ./src/customerauth -count=1` passed.
- OpenAPI/runtime route-contract verification passed.
- `go test ./...` and `go vet ./...` passed.
- The enabled service is active on `127.0.0.1:18080`; local and public
  liveness/readiness, Swagger UI, and the live 137-operation OpenAPI passed.
- The protected User App charger route returned `401` without customer
  credentials, and the post-restart warning journal was empty.
- The capacity-field residue scan and `git diff --check` passed.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

### Split User App hub and charger opening-hour semantics

- Replaced the ambiguous User App hub field
  `twenty_four_seven_open_status` with `open_24_hours`.
- `CustomerCharger.twenty_four_seven_open_status` now maps directly to the
  CPO-owned charger field, while `hub_open_24_hours` separately maps to the
  attached hub field.
- This is an intentional User App contract correction: consumers must switch
  hub reads to `open_24_hours` and must not infer charger opening status from
  the hub value.

Verification:

- `go test ./src/customerauth -count=1` passed, including opposing hub/charger
  opening-hour values in the serialized projection.
- OpenAPI/runtime route-contract verification passed.
- `go test ./...`, `go vet ./...`, the opening-hour residue scan, and
  `git diff --check` passed.
- Built and rehosted source revision `9b57f20` on the development VPS without a
  new migration; the live database remains at migration twenty-seven.
- The enabled service is active on `127.0.0.1:18080`; local and public
  liveness/readiness, Swagger UI, and the live 137-operation OpenAPI passed.
- The protected User App charger route returned `401` without customer
  credentials, and the post-restart warning journal was empty.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

## 2026-08-07

### Rehosted CPO connector-capacity contract revision

- Built and rehosted revision `86170d3` on the development VPS without a new
  migration; the application remains on migrations through twenty-seven.
- The running process matches the installed binary SHA-256 and embeds
  `86170d32f6114f480833a4cd6388d058ed0983ca` with
  `vcs.modified=false`.
- The live 137-operation OpenAPI CPO `Connector` response schema exposes
  `connector_total_capacity`, matching the runtime response projection.

Verification:

- `evcmsnew-dev.service` is active and enabled.
- Loopback and public liveness/readiness checks passed.
- Running-binary SHA/VCS identity, live OpenAPI operation count, live CPO
  connector schema, and post-start fatal-error scan passed.
- No migration file changed from the previously deployed revision.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

### Reconciled CPO connector-capacity response contract

- Reconciled the CPO connector response contract with the runtime projection:
  connector create/update request payloads continue to use `total_capacity`,
  while CPO connector response objects use `connector_total_capacity`.
- Updated canonical OpenAPI, the administrative HTTP contract, current project
  state, development-hosting guidance, and the active CPO work item.
- No database migration or runtime behavior change was introduced by this
  documentation reconciliation.

Verification:

- OpenAPI/runtime route-contract verification passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.


### Corrected User App connector-capacity documentation

- Corrected the User App hub-detail and charger-discovery descriptions to use
  the runtime and User App OpenAPI field name `connector_total_capacity`.
- This preserves the distinct CPO administrative connector contract, which
  uses `total_capacity`; no CPO-facing code or contract was changed.

Verification:

- Checked the User App FE handoff and `CustomerConnector` OpenAPI schema
  against the documented field name.
- `git diff --check` passed.
- Updated the documentation verifier's route and 137-operation expectations;
  the PowerShell execution remains unavailable on this Ubuntu host.

### Rehosted CPO customer-directory and charger contract release

- Rebuilt and rehosted revision `79683f0` without a new migration; migration
  27 remains current.
- Published the tenant-scoped, read-only CPO customer list/detail operations,
  the charger `hub_name` projection, and the consistent CPO API
  `total_capacity` connector field.
- Reconciled the customer-list query contract with runtime validation: search
  is bounded to 200 characters, `status` accepts `ACTIVE` or `BLOCKED`, and
  `limit` is 1–200 with a default of 50.

Verification:

- OpenAPI/runtime parity, full Go tests, `go vet`, formatting, and diff checks
  passed.
- Service active/enabled state, loopback listener, local/public liveness and
  readiness, 137-operation live OpenAPI, Swagger UI, protected CPO
  customer-list route, migration ledger, and post-start journal passed.
- The PowerShell documentation verifier is unavailable on this host.

### Implemented CPO Customer Directory Endpoints

- Added read-only endpoints for CPO administrators to view their registered app
  customers: `GET /api/v1/cpo/customers` for a paginated and searchable list,
  and `GET /api/v1/cpo/customers/{customer_id}` for a single customer view.
- Updated the administrative API contract, CPO backend handoff, and Superadmin
  handoff to document the new endpoints, their read-only nature, and the strict
  tenant boundary.
- The OpenAPI specification was updated to include these new operations.

Verification:

- Documentation verification and `git diff --check` passed.
## 2026-08-07

### Assigned CPO network, pricing, and integration ownership

- Created `WI-20260807-cpo-network-pricing-operations` to assign Abhranil Pal
  ownership of the CPO network (hub/charger), pricing (GST/tariff), and
  integration-credential backend surfaces.
- The work item clarifies that platform CPO control, authentication, and
  customer-facing APIs remain owned by their respective work items.

Verification:

- Documentation verification and `git diff --check` passed.

### Made the repository operating contract self-contained

- Expanded the root `AGENTS.md` from a narrow documentation/verification note
  into the repository's standalone operating contract. It now records the
  actual Go module ownership, PostgreSQL migration safety, configuration and
  runtime references, CMS/HAL and tenant boundaries, contract/recovery rules,
  active-work coordination, operational/Git approval limits, and honest
  verification requirements.
- Added the root operating contract to the documentation map so a new agent can
  discover the authoritative workflow without relying on machine-wide
  bootstrap instructions.

Verification:

- Route/OpenAPI parity, full Go tests, `go vet`, and whitespace checks passed.
- The PowerShell documentation verifier was not run because this Ubuntu host
  does not provide `pwsh`; its documented validation remains unverified here.

### Registered active ownership for platform, User App, and operations work

- Added the repository-native active-work ledger and registered Anubhab Dey as
  the owner of SuperAdmin and User App backend work, shared authentication,
  mail, and notification infrastructure, and related hosting, database, and
  DNS operations.
- Registered Anubhab as the owner of HAL-to-CMS and CMS-to-HAL communication
  changes, while assigning CMS-side receipt and handling to the respective
  CMS surface owner.
- The work item is an ownership record only; it does not report a code,
  database, DNS, hosting, or deployment change.

Verification:

- Documentation verification and whitespace checks passed.

### Rehosted CPO assignment and Swagger grouping release

- Rebuilt and rehosted revision `5ed7cb8` without a new migration; migration
  27 remains current.
- Confirmed the newly exposed CPO charger `assigned` response projection is
  described consistently in the administrative contract and CPO agent handoff:
  it mirrors whether a same-CPO hub is attached through `hub_id`.
- Recorded the live 135-operation Swagger grouping for CPO account and
  notifications, network, pricing and tax, and integrations.

Verification:

- OpenAPI/runtime parity, full Go tests, `go vet`, formatting, and diff checks
  passed.
- Service active/enabled state, loopback listener, local/public liveness and
  readiness, live OpenAPI, Swagger UI, protected CPO charger-list route, and
  post-start journal passed after rehost.
- The PowerShell documentation verifier is unavailable on this host.

### Organized CPO Swagger operations

- Split the flat CPO Swagger tag into four consistently prefixed sections:
  Account & Notifications, Network, Pricing & Tax, and Integrations.
- Kept all CPO-plane operations together while leaving platform and User App
  endpoints in their existing separate Swagger sections.

Verification:

- Documentation contract verification, CPO tag inventory, and diff checks
  passed. Go tests remain skipped by the requested workspace policy.

## 2026-08-06

### Rehosted published charger-image and hub-delete release

- Rebuilt and rehosted revision `9894b26` without a new migration; migration
  27 remains current.
- Confirmed the User App charger-image route and CPO hub-delete route are
  registered, protected, and present in the live 135-operation OpenAPI/Swagger
  contract.
- Formatted the merged CPO source files without changing their behavior.

Verification:

- OpenAPI/runtime parity, full Go tests, `go vet`, formatting, and diff checks
  passed.
- Service active/enabled state, loopback listener, local/public liveness and
  readiness, live route protection, OpenAPI, and Swagger UI passed after
  rehost.
- The PowerShell documentation verifier is unavailable on this host.

### Reconciled merged CPO network routes

- Repaired the duplicated `deleteCPOHub` OpenAPI operation introduced on the
  merged mainline and restored one canonical operation.
- Completed route-inventory and human-contract coverage for CPO charger-image
  reads and hub deletion; the source OpenAPI now has 135 operations.

Verification:

- Documentation verification, static analysis, and route/OpenAPI inventory
  reconciliation are pending after the merge; Go tests remain skipped by the
  requested workspace policy.

### Added authenticated User App charger images

- Added `GET /api/v1/app/chargers/{charger_id}/image`, using the public
  six-character charger ID and the same customer/CPO/published-hub boundary as
  charger detail.
- Charger projections now expose an optional relative `charger_image_url`, not
  the stored filesystem path. The route only serves regular JPEG, PNG, GIF, or
  WebP files from the established upload directory and requires the normal
  Bearer and app-ID headers.
- Updated OpenAPI, the User App handoff, human API contract, plan/state, and
  operation-count verifier for the 133-operation source tree.

Verification:

- Pending focused static checks after implementation; Go tests remain skipped
  by the requested workspace policy.

### Rehosted charger-status and User App projection release

- Rebuilt and rehosted revision `9ccdff2` after confirming migration 27 and
  its replacement charger/connector status constraints were already applied.
- Corrected the remaining charger-create and new-connector defaults to the
  migration-27 CMS status values: new chargers are `INACTIVE`; new connectors
  are `ACTIVE`.
- Reconciled the CPO network plan and administrative contract so the dedicated
  CPO charger-status route is documented as static CMS administration, never
  HAL-backed live availability.

Verification:

- OpenAPI/runtime parity, full Go tests, `go vet`, formatting, and diff checks
  passed.
- Service active/enabled state, loopback listener, local/public liveness and
  readiness, live OpenAPI operations, Swagger UI, migration ledger, and live
  status constraints passed after rehost.
- The PowerShell documentation verifier is unavailable on this host.

### Consolidated current charger data into User App and repaired route contracts

- Added DB-backed static CMS `status` to User App charger and connector
  projections, and added connector `connector_total_capacity`; `availability`
  remains the separate HAL-owned live field and is still always `UNKNOWN`.
- Repaired migration 27's legacy-status conversion so every previously allowed
  charger/connector status maps to a value accepted by the replacement
  constraint before that constraint is added.
- Added the already registered CPO hub-publication and charger-status routes to
  canonical OpenAPI and the human CPO contract. Charger-status endpoints use
  the internal charger UUID, while the established CPO charger detail route
  continues to use the public six-character charger ID.
- Updated the CPO agent handoff, User App handoff, project plan/state, and
  documentation operation-count verifier for the 132-operation source tree.

Verification:

- Pending focused static checks after the full contract update; Go tests remain
  skipped by the requested workspace policy.

### Clarified User App favorite charger identifier

- Documented that User App favorite-charger mutations accept the six-character
  `CustomerCharger.charger_id`, not the internal UUID in `CustomerCharger.id`.
  This aligns the frontend handoff, human HTTP contract, and OpenAPI with the
  runtime validator.

Verification:

- Documentation verifier and whitespace check passed; Go tests remain skipped
  by the requested workspace policy.

### Rehosted current main and reconciled CPO network contracts

- Rebuilt and restarted the current `782dd7b` deployment after confirming
  migration 26 was already present; the ignored service environment now
  supplies the public OCPP WebSocket host for charger response construction.
- Corrected the documented `GET /api/v1/cpo/hubs/{hub_id}` response to include
  its paginated charger list and cursor query contract.
- Corrected the OpenAPI CPO charger response shape and documented the emitted
  OCPP connection URL fields. The current development OCPP route is plain
  WebSocket only, so consumers must not use the returned `wss://` address until
  TLS is configured for that host.

Verification:

- Service active/enabled state, loopback listener, process configuration,
  local/public liveness, and local/public readiness passed after rehost.

### Rehosted User App route split and connector contract update

- Applied migration 26, which removes obsolete connector `max_current` and
  `max_voltage` columns; `connector_total_capacity` remains the supported
  connector capacity field.
- Deployed the separated User App route topology: authenticated resources are
  under `/api/v1/app`, while credential/session operations remain under
  `/api/v1/app/auth`; the former resource URLs are intentionally absent.
- Reconciled the OpenAPI and User App handoff connector schemas with the runtime
  removal of current/voltage fields.
- Built and rehosted revision `782dd7b`.

Verification:

- Public readiness, loopback listener, service active/enabled state, migration
  ledger, OpenAPI route presence, and the new-vs-retired resource route status
  were verified after rehost.
- Full Go tests, `go vet`, OpenAPI/runtime parity, and diff checks passed.
- The PowerShell documentation verifier is unavailable on this host.

### Rehosted charger part-time field correction

- Corrected CPO charger updates to write the actual
  `twenty_four_seven_open_status` database column.
- Aligned customer network response contracts and the User App handoff with
  the runtime `twenty_four_seven_open_status` field; the `open_24_hours` query
  filter remains unchanged.
- Built and rehosted revision `3645529` without a new migration.

Verification:

- Full Go tests, `go vet`, OpenAPI/runtime parity, and diff checks passed.
- PostgreSQL remains through migration 25; service readiness passed after
  rehost.
- The PowerShell documentation verifier is unavailable on this host.

### Clarified User App environment base URLs

- Expanded the User App frontend/AI-agent handoff with explicit local,
  shared-development, and approved-deployment API-origin rules.
- Defined deterministic `VITE_EV_CMS_API_ORIGIN` selection, shared
  `/api/v1` base derivation, separate `/app/auth`, `/cpo`, and `/platform`
  route groups, health/docs URLs, CPO app-ID separation, and the prohibition
  on using `0.0.0.0` as a browser URL or guessing a production hostname.

### Corrected User App handoff against routed behavior

- Reclassified the handoff as the full User App HTTP surface, corrected the
  customer status union to `ACTIVE | BLOCKED`, and removed the invalid notion
  that an API origin can include a path.
- Corrected wallet documentation to reflect the implemented Razorpay order and
  verification routes, durable wallet credit, idempotency, provider failure
  responses, and still-unsupported refund/billing/reconciliation surfaces.
- Added exact hub/charger/wallet pagination and near-me filter rules, and
  corrected tariff wording so hub-price reads cannot select charger-scoped
  tariffs.
- Established the separate User App namespace and credential-root derivation
  used by the canonical `/api/v1/app` and `/api/v1/app/auth` route split.

### Separated User App resource routes from credential routes

- Moved authenticated User App resources to `/api/v1/app`: current customer
  bootstrap/profile, published hubs and chargers, pricing, favorites, wallet
  reads, and Razorpay recharge.
- Kept credential and session operations under `/api/v1/app/auth`: signup,
  login, refresh, password recovery/change, session list/revocation, and
  logout. Updated route checks, OpenAPI, human contracts, guides, integration
  guidance, project state, plan, and the User App handoff in the same slice.
- This is intentionally not an alias release: the former `/api/v1/app/auth`
  resource URLs are removed and frontend clients must use the canonical
  `/api/v1/app` resource paths in the same deployment.

## 2026-08-05

### Rehosted optional charger metadata behavior

- Charger vendor/model fields now remain optional across CPO create/update and
  customer network response projections, with omitted values represented as
  null.
- Removed an unused `CHARGER_CONNECTION_URL` example setting and normalized
  optional metadata before pointer persistence.
- Built and rehosted revision `0b344f9` without a new migration.

Verification:

- Focused OpenAPI/runtime parity, full Go tests, and `go vet` passed.
- PostgreSQL remains through migration 25; public readiness and service state
  passed after rehost.
- The PowerShell documentation verifier is unavailable on this host.

### Rehosted charger capacity contract update

- Applied migration 25, removing the obsolete charger-level `total_capacity`
  column; connector-level capacity remains supported.
- Reconciled the administrative API documentation and OpenAPI schema, including
  the missing `TariffListResponse` component referenced by the runtime contract.
- Built and rehosted revision `dfc5039`; the live API remains the 129-operation
  contract.

Verification:

- Focused OpenAPI/runtime parity, full Go tests, and `go vet` passed.
- PostgreSQL reports migration 25 applied and no charger `total_capacity` column.
- Service is active/enabled; public readiness and `/docs/` were verified.
- The PowerShell documentation verifier is unavailable on this host.

### Rehosted optional charger metadata persistence

- Applied migration 24 from a mode-0600 rollback dump so charger `vendor` and
  `model` persistence columns accept null values for incomplete projections.
- Built and rehosted revision `4e06f10`; the live API remains the 129-operation
  contract and no new HTTP route was introduced.

Verification:

- Migration 24 was applied and PostgreSQL reports both columns nullable.
- Application tests and vet completed before rehost.
- Public readiness and service state were checked after rehost.
- The PowerShell documentation verifier is unavailable on this host; static
  documentation checks and Go route/OpenAPI verification were used instead.

### Fixed subscription history table resolution

- Corrected `CPOSubscriptionHistory` to use the existing singular
  `cpo_subscription_history` table created by migration seven. GORM had been
  defaulting to `cpo_subscription_histories`, causing platform subscription
  commands to fail with PostgreSQL `42P01` after deployment.
- No database migration is required; this is an application mapping correction
  compatible with existing subscription data.

Verification:

- `git diff --check` and documentation verification pass.
- Go tests were skipped per the current instruction.

## 2026-08-05

### Removed the local `.env` from version control

- Removed the tracked root `.env` from the Git index while preserving the local
  file on disk for the current environment.
- Expanded ignore rules for root `.env.*` files while explicitly retaining the
  checked-in `.env.example` template as the safe configuration inventory.
- This is a forward cleanup only: it does not rewrite historical commits. A
  deployment or checkout can recreate its ignored `.env` from `.env.example`
  and inject real values locally or through process environment variables.

Verification:

- Confirmed `.env` is no longer tracked and remains present locally.
- Confirmed `.env.example` remains tracked and `.env.local`-style files are
  ignored.
- Full application verification is unchanged; no secret contents were printed
  or added to the repository.

## 2026-08-05

### Rehosted customer network and Razorpay wallet release

- Applied migrations 21 and 22 from a mode-0600 rollback dump. The live
  database had one customer and two hubs; recharge tables started empty.
- Built and rehosted revision `c33da86`. The enabled service is active and
  serves the 129-operation contract through Caddy.
- Verified public readiness and synchronized the deployment, project-state,
  CPO, SuperAdmin, and User App documentation with the live release.

## 2026-08-05

### Implemented User App Razorpay wallet recharge

- Added User App order creation and checkout verification using the existing
  encrypted CPO Razorpay integration credentials; no CPO or Superadmin payment
  API was added.
- Added migration 22 and durable models for recharge orders, provider payment
  attempts, future refund records, sanitized provider payloads, and payment
  signature evidence. A captured payment credits the customer wallet and
  creates one linked completed ledger transaction atomically and idempotently.
- Used `github.com/razorpay/razorpay-go` for provider order/payment calls and
  standard-library HMAC verification for the checkout signature.
- Updated OpenAPI/Swagger, route parity, the exhaustive HTTP contract, Razorpay
  integration record, User App frontend handoff, development plan, customer
  app plan, and project state. Refund execution, webhooks, settlement
  reconciliation, RFID, and HAL remain deferred.

Verification:

- Focused customer-auth, model, migration, and route tests pass.
- `scripts/verify-docs.ps1` and OpenAPI/runtime parity pass after the operation
  count was updated to 129.
- `go test ./...`, `go vet ./...`, and `git diff --check` pass.
- Disposable PostgreSQL lifecycle verification remains intentionally deferred
  by decision and is not claimed as passed.

## 2026-08-05

### Implemented User App charger search and wallet read APIs

- Added authenticated charger search/filter and bounded near-me reads over
  published hubs, supporting text, connector type, power, opening-hours, and
  latitude/longitude radius filters. Location results include calculated
  distance and do not claim HAL-backed live availability.
- Added authenticated wallet balance and descending keyset-paginated wallet
  transaction history projections scoped from the trusted customer principal.
  Internal idempotency keys and provider credentials are not exposed.
- Kept wallet mutation, Razorpay recharge, refunds, charging-session history,
  RFID, and HAL control out of this slice.
- Updated OpenAPI/Swagger, route parity, User App handoff, exhaustive HTTP
  contract, development plan, customer-app plan, and project state.

Verification:

- `go test ./src/customerauth -count=1` passes.
- `go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1` passes.
- `scripts/verify-docs.ps1` passes.
- Disposable PostgreSQL lifecycle verification remains intentionally deferred
  by decision and is not claimed as passed.

## 2026-08-05

### Corrected User App tariff precedence

- Clarified and corrected the resolver to use the three entity tiers: matching
  UserGroup tariff, then generic charger tariff, then generic hub tariff.
- A matching UserGroup tariff now always beats a generic charger tariff. When
  both group/charger and group/hub rows apply, charger scope is only a
  same-tier specificity tie-breaker.
- Updated the User App, CPO, OpenAPI, HTTP contract, plan, and project-memory
  documentation.

Verification:

- Database-free precedence tests cover UserGroup over generic charger and
  generic charger over generic hub.

## 2026-08-05

### Implemented User App informational tariff resolution

- Added authenticated hub and public-charger price reads over published
  customer network resources.
- Added deterministic server-side precedence: matching User Tariff (the
  existing customer `UserGroupID`) over charger tariff over hub tariff, with
  charger-specific scope winning within the User Tariff and generic tiers.
- Added exact decimal price/GST projections and an explicit `UNAVAILABLE`
  response when no eligible tariff or active referenced GST exists. The result
  is informational and does not contact HAL or commit a charging price.
- Updated OpenAPI/Swagger, runtime route tests, exhaustive HTTP contract, User
  App handoff, CPO handoff, development plan, and project state.

Verification:

- Database-free tariff precedence tests and focused customer-auth/route tests
  pass.
- Disposable PostgreSQL lifecycle verification remains intentionally deferred
  by decision and is not claimed as passed.

### Deferred disposable PostgreSQL lifecycle testing for the User App workstream

- The current implementation workstream will skip disposable PostgreSQL
  lifecycle execution until explicitly reactivated.
- Stateful tests remain in the repository and must not be described as passed;
  source, database-free, contract, documentation, and full Go checks remain the
  verification path for the non-HAL User App slices.
- Work continues only on `anubhab-work`; `main` is not updated by this
  workstream.

### Implemented published User App network discovery

- Added migration 21 and CPO ADMIN-controlled `customer_visible` hub
  publication, defaulting to false on create and supporting explicit update.
- Added authenticated customer routes for published hub listing/detail and
  public charger-ID detail. Queries derive CPO/customer scope from the
  validated principal, hide unpublished/unassigned/cross-CPO resources, and
  return safe projections only.
- Kept HAL out of this slice: charger and connector availability is explicitly
  `UNKNOWN`, with no claim of live state or OCPP connectivity.
- Updated OpenAPI/Swagger, runtime route tests, exhaustive HTTP contract, User
  App handoff, CPO handoff, schema/workflow docs, development plan, and state.

Verification:

- `go test ./src/customerauth ./src/routes ./src/cpo -count=1` passes.
- `go test ./...`, `go vet ./...`, `scripts/verify-docs.ps1`, and
  `git diff --check` pass.
- Disposable PostgreSQL lifecycle verification is intentionally deferred by
  decision and is not claimed as passed.

### Implemented customer favorites over published network data

- Added `GET /favorites` with independent bounded hub and charger cursors.
- Added idempotent `PUT`/`DELETE` favorite routes for published same-CPO hubs
  and attached public-ID chargers, with composite tenant ownership and audit
  actions for actual mutations.
- Unpublished resources are omitted from favorite reads without deleting or
  leaking the customer's durable saved intent; no HAL call is made.
- Updated OpenAPI/Swagger, route tests, exhaustive HTTP contract, User App
  handoff, CPO handoff, development plan, and project state.

Verification:

- Focused customer-auth and route/OpenAPI checks pass.
- Disposable PostgreSQL lifecycle verification is intentionally deferred by
  decision and is not claimed as passed.

### Fixed PostgreSQL-invalid customer signup advisory-lock key

- Replaced the NUL-containing signup lock key with a text-safe
  `customer-signup:<cpo-id>:<normalized-email>` key and
  `hashtextextended(?, 0)`, preserving transaction-scoped serialization.
- Committed as `70da3e6` on `anubhab-work` and published to `main` as
  `cf9a000` because signup verification was failing at PostgreSQL text input.

## 2026-08-05

### Implemented customer self-service profile editing

- Added authenticated `PATCH /api/v1/app/auth/profile` using the validated
  CPO-local customer principal and matching `X-CPO-App-ID`.
- Restricted mutation to trimmed full name and normalized phone; omitted phone
  preserves the value and explicit JSON `null` clears it.
- Added transactional customer update and `CUSTOMER_PROFILE_UPDATED` audit
  evidence containing changed field names only, with no old/new PII values.
- Updated runtime route tests, OpenAPI, the exhaustive HTTP contract, User App
  frontend handoff, CPO agent handoff, development plan, and project state.

Verification:

- `go test ./src/customerauth ./src/routes -count=1` passes.
- OpenAPI/runtime parity and Swagger/raw-schema route verification passes.
- PostgreSQL lifecycle verification remains pending because no explicitly
  disposable `TEST_DATABASE_URL` is configured.
### Removed the local `.env` from version control

- Removed the tracked root `.env` from the Git index while preserving the local
  file on disk for the current environment.
- Expanded ignore rules for root `.env.*` files while explicitly retaining the
  checked-in `.env.example` template as the safe configuration inventory.
- This is a forward cleanup only and does not rewrite historical commits.

## 2026-08-04

### Rehosted CPO-local customer authentication

- Applied migration 20 after a rollback dump; the database had zero customer
  rows, satisfying the migration's explicit safety precondition.
- Built and rehosted revision `be6fd34`. The enabled service is active with
  zero restarts and serves the 113-operation contract through Caddy.
- Verified loopback/public health and readiness, Swagger/OpenAPI, migration 20,
  customer-account zero state, and the existing protected service boundary.

## 2026-08-04

### Implemented CPO-local customer authentication with full route parity

- Migration 000020 moves app email/password/profile/verification/lockout state
  onto `customers`, removes the global-user link, and adds dedicated durable
  customer challenge, session, and rotating refresh-token tables with
  composite CPO ownership.
- Preserved the full app auth route surface: verified signup/resend,
  password-plus-mail-OTP login/resend, encrypted access tokens, refresh
  rotation/reuse revocation, enumeration-safe recovery/reset, authenticated
  password change, `me`, session list/revoke, logout, and logout-all.
- Access-token subject and trusted app principal now use `customer_id`.
  `CurrentUserID` and `me.user` remain compatibility shapes whose ID equals the
  customer ID and never identifies an administrative `users` row.
- CPO suspension now revokes dedicated customer sessions as well as staff
  sessions. The CPO user point lookup is staff-membership-only and no longer
  depends on `customers.user_id`.
- Customer signup/login/recovery outbox jobs retain CPO correlation while
  leaving the administrative `user_id` empty.
- Updated tests, OpenAPI, exhaustive HTTP/workflow contracts, schema/state,
  development plans, CPO-agent guidance, the standalone User App frontend
  handoff, and ADR supersession status.

Verification:

- `go test ./...` passes without a database URL; PostgreSQL lifecycle tests
  compile and skip because `TEST_DATABASE_URL` is not configured.
- Documentation verification, OpenAPI/runtime route parity, `go test ./...`,
  `go vet ./...`, and `git diff --check` pass on the reconciled source.

### Reconciled current main with the customer-auth implementation

- Merged the latest remote `main`, retaining its CPO hub changes and read-only
  CPO subscription endpoint alongside the CPO-local customer-auth boundary.
- Corrected the incoming subscription query to return only the current
  non-terminal record rather than an arbitrary historical record, made its
  plan response contract required, and documented that CPO access is read-only
  and never controlled by subscription state.
- Reconciled the machine-contract verifier and current-source documentation to
  the resulting 113 routed OpenAPI operations; the deployed development
  revision remains separately documented at 112 operations.

Verification:

- Documentation verification, OpenAPI/runtime route parity, `go test ./...`,
  `go vet ./...`, and `git diff --check` pass after the merge.
- Stateful PostgreSQL customer-auth and subscription-read lifecycle execution
  remains pending because no explicitly disposable `TEST_DATABASE_URL` is
  configured.

### Corrected customer identity direction

- Approved CPO-scoped customer accounts: customer email, password, profile,
  recovery, sessions, and refresh lineage are separate per CPO; global `users`
  remain for Superadmin/CPO staff only. The same email/password combination is
  allowed across CPOs but later changes remain scoped to one account.
- Superseded the uncommitted global-user profile implementation pending the
  additive customer-account migration and complete customer-auth refactor.

### Superseded global-user customer-profile prototype

- The uncommitted profile prototype used the former global `users` customer
  identity and is superseded by ADR 0013. It must be refactored to the
  CPO-scoped customer account before publication.

Verification:

- The former prototype's focused checks passed, but they do not verify the
  approved CPO-scoped customer-account behavior and are not release evidence.

### Approved customer-app experience plan

- Recorded the end-to-end customer-app implementation sequence in
  `docs/plans/customer-app-experience.md`: customer profile, published network
  discovery, favorites, server-side tariff display, future access credentials,
  CMS/HAL charging lifecycle, wallet/billing/history, and later customer
  notifications/realtime.
- Retained `X-CPO-App-ID` as the required routing header on every existing and
  future `/api/v1/app/...` request, including public signup. The header is
  routing metadata, never authority; protected customer routes continue to
  compare it to the validated customer principal.
- Recorded that the CPO-owned network needs an explicit default-false
  customer-publication control before discovery can safely expose a hub.

Verification:

- Repository routing and existing contract references were inventoried. No
  runtime endpoint, schema, OpenAPI operation, migration, or deployed behavior
  changed in this planning slice.

### Rehosted CPO hub configuration revision

- Added migration nineteen to reconcile development databases that had already
  recorded historical hub follow-up migrations while retaining `hub_id NOT NULL`.
  The forward migration makes independent charger inventory possible; its
  rollback refuses to restore `NOT NULL` while unassigned chargers exist.
- Built and rehosted revision `4502934` after a mode-0600 PostgreSQL rollback
  dump. The service is enabled, active, zero-restart, and serves the 112-operation
  contract through Caddy at `dev-evcmsnew.transev.site`.
- Verified loopback/public live and ready health, Swagger/OpenAPI, nullable
  `chargers.hub_id`, migration nineteen, protected CPO routes, required workers,
  and no startup fatal/panic/error records.

### Reconciled CPO hub configuration and independent charger lifecycle

- Reconciled the incoming CPO hub changes against the authoritative platform
  baseline: chargers may now be created independently with no `hub_id`, then
  attached or moved only through same-CPO, transactionally audited operations.
- Added `POST /api/v1/cpo/hubs/{hub_id}/chargers` as the explicit attachment /
  reassignment contract. Repeating the current target is side-effect free;
  tariff-scope cascades that would overlap active schedules roll back with
  `409 tariff_schedule_conflict`.
- Consolidated undeployed sanctioned-load work into migration sixteen with a
  database non-negative constraint, upgrade-time removal of the legacy hub
  `NOT NULL`, a rollback guard for independent chargers, and application
  validation. Removed the redundant OCPP-identity-index and split
  sanction-load migrations before deployment, and restored rejection of empty
  migration files.
- Updated the CPO API, OpenAPI, workflow, backend handoff, schema, plan, and
  project-state documentation. The source contract now has 112 operations;
  the development VPS remains on deployed revision `bb555b9`, migration
  fifteen, and 111 operations.

Verification:

- `scripts/verify-docs.ps1`, focused runtime/OpenAPI parity, `go test ./...`,
  `go vet ./...`, and `git diff --check` passed.
- The targeted PostgreSQL assignment/reassignment lifecycle test remains
  intentionally skipped because `TEST_DATABASE_URL` is not configured; no
  non-disposable database was touched.

### Completed the SuperAdmin frontend API handoff

- Reconciled `docs/SUPERADMIN_FRONTEND_HANDOFF.md` with the current 111-operation
  OpenAPI contract, expanding the SuperAdmin inventory from the previously
  listed 28 operations to all 66 platform-consumable operations.
- Added frontend request/response types and workflow guidance for platform
  administrator governance, security operations, safe mail administration,
  announcements, notifications, overview/status, and manual subscriptions.
- Corrected password-recovery screen guidance and documented subscription
  idempotency, explicit lifecycle control, and the absence of provider billing,
  feature keys, entitlements, and automatic subscription behavior.

Verification:

- `scripts/verify-docs.ps1` passed.
- `git diff --check` passed.

### CPO user lookup and tariff scheduling deployed

- Reviewed the reconciled `bb555b9` merge against the deployed `396bae5`
  baseline, including the tenant-safe CPO user point lookup, tariff effective
  dates, migration fifteen, OpenAPI, and frontend/backend guidance.
- Confirmed the migration preflight against the live database: migration
  fourteen was current, `btree_gist` was available, and no tariff row existed
  to violate the new active-scope exclusion constraint.
- Created the validated mode-0600 rollback dump
  `/tmp/devevcmsnewdb-pre-bb555b9.dump`, preserved the previous binary as
  `builds/evcmsnew.pre-bb555b9`, applied migration fifteen, installed the clean
  candidate, and rehosted `evcmsnew-dev.service` through the shared handler.

Verification:

- Migration fifteen installed both effective-date columns, both constraints,
  both date indexes, and `btree_gist`; all four CPOs were preserved.
- Focused migration/CPO tests, runtime/OpenAPI parity, documentation contract
  verification, `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- The installed binary matches the clean `bb555b9` candidate. Systemd is
  enabled and active with zero restarts; loopback/public readiness, Swagger,
  the live 111-operation OpenAPI, request-ID delivery, protected routes,
  retired billing routes, required workers, and the startup journal passed.
- No live authenticated CPO mutation or mail delivery was used for deployment
  verification. Disposable PostgreSQL lifecycle coverage remains pending.

### Explicit CPO contribution merge reconciled on `anubhab-work`

- Merged `abhranil_ev_cms_backend_new` into `anubhab-work` while retaining the
  deployed Superadmin and manual-subscription baseline as authority.
- Kept the tenant-scoped CPO user point lookup, but made it a generic-404-safe
  membership/customer relationship lookup rather than a CPO directory.
- Renumbered the incoming undeployed tariff migration to fifteen, restored
  mandatory hub scope, removed stale specialized tariff contracts, and replaced
  race-prone application overlap checks with a PostgreSQL `tstzrange` exclusion
  constraint and a preflight for existing overlapping active tariffs.
- Updated CPO/API/OpenAPI/schema/plan/project-state documentation. The source
  contract has 111 operations; migration fifteen and the corresponding binary
  are not deployed.

### Complete Superadmin surface deployed

- Completed the source-side platform Superadmin surface for administrator
  governance, locked-identity/session security operations, safe mail-job
  administration, announcements, durable platform/CPO notifications, bounded
  overview, and service status.
- Added additive migration fourteen for platform authority status fields,
  `CANCELED` mail jobs, administrator templates, announcements, and durable
  notification rows.
- Expanded the canonical OpenAPI contract from 87 to 110 operations and
  updated the route parity fixture, frontend handoff, administrative API,
  realtime, mail, integration, guide, plan, project-state, and ADR records.
- Added database-free validation/privacy tests and migration-content coverage;
  mail-job responses are verified not to expose stored error text.

Verification:

- `scripts/verify-docs.ps1` passed.
- `go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1` passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- Stateful PostgreSQL lifecycle verification remains pending because no
  disposable `TEST_DATABASE_URL` is configured.

Deployment verification:

- Created the mode-0600 rollback dump
  `/tmp/devevcmsnewdb-pre-396bae5.dump` and preserved the prior binary as
  `builds/evcmsnew.pre-396bae5`.
- Confirmed migration thirteen before applying migration fourteen, then
  verified the new platform-authority columns and announcement/notification
  tables.
- Rehosted `evcmsnew-dev.service` with `396bae5`. Systemd is enabled and
  active with zero restarts; loopback/public liveness and readiness, Swagger
  UI, and the live 110-operation OpenAPI passed. No recent startup error or
  panic was recorded.

### Feature-key runtime removed and deployed pending a module catalog

- Added migration thirteen to return `subscription_plan_entitlements` and
  `cpo_entitlement_overrides` to `retired_commercial` while preserving their
  rows. Plans, versions, CPO subscriptions, and transition history remain
  active.
- Removed feature-key/entitlement payload fields, models, routes, events,
  OpenAPI operations, frontend guidance, and false feature-gating claims.
- `cpos.status` remains the only whole-CPO service control. A future feature
  model requires an approved fixed module catalog and server-side enforcement.

Verification and deployment:

- Documentation verification, migration static checks, subscription/model/
  route validation, `go test ./...`, `go vet ./...`, and `git diff --check`
  passed before activation. The disposable PostgreSQL lifecycle remains
  unexecuted without `TEST_DATABASE_URL`.
- Created the mode-0600 rollback dump
  `/tmp/devevcmsnewdb-pre-9b508ef.dump` and preserved the prior binary as
  `builds/evcmsnew.pre-9b508ef`.
- Confirmed migration twelve before applying migration thirteen, verified both
  feature-key tables in `retired_commercial` with their rows preserved, and
  confirmed active subscription plan/version/subscription/history tables remain
  in `public`.
- Rehosted `evcmsnew-dev.service` with `9b508ef`. Systemd is enabled and active
  with zero restarts; loopback/public liveness and readiness, Swagger UI, and
  the live 87-operation OpenAPI passed. No recent startup error or panic was
  recorded.

### Manual platform subscriptions restored without a provider

- Added migration twelve to restore only subscription plans, immutable
  published versions, CPO subscriptions/history, and entitlement overrides
  from `retired_commercial`. Platform billing tables remain retired.
- Added platform-superadmin-only plan/catalog, manual issue/renew/change/status
  commands, history, and entitlement overrides. UUIDs, plan versions,
  timestamps, audits, events, and subscription idempotency evidence are server
  generated.
- Deliberately excluded provider, checkout, invoice/payment, webhook, mail,
  scheduled plan change, automatic renewal, trial completion, expiry,
  cancellation, lifecycle worker, and CPO-access coupling.
- Added ADR 0012, the manual-subscription plan/contract, OpenAPI coverage,
  frontend/backend handoff updates, and migration/manual-lifecycle tests.

Verification:

- Documentation verification, migration static checks, manual-subscription
  validation, route/OpenAPI parity, `go test ./...`, `go vet ./...`, and
  `git diff --check` passed.
- The database-backed manual lifecycle test is registered but skipped without
  an explicitly configured disposable `TEST_DATABASE_URL`.

Deployment verification:

- Created the mode-0600 rollback dump
  `/tmp/devevcmsnewdb-pre-c50bc49.dump` and preserved the prior binary as
  `builds/evcmsnew.pre-c50bc49`.
- Confirmed migration eleven before applying migration twelve, then verified
  migration twelve, all six restored subscription tables in `public`, and
  both retired automatic-lifecycle workers still disabled.
- Rehosted `evcmsnew-dev.service` with the clean `c50bc49` binary. Systemd is
  enabled and active with zero restarts; loopback/public liveness and
  readiness, Swagger UI, and the live 90-operation OpenAPI passed. No recent
  startup error or panic was recorded.

## 2026-08-03

### Password-recovery outbox payload forwarding fixed and deployed

- Removed the lossy OTP-only enqueue wrapper and made every OTP producer call
  the canonical mail enqueue operation with the complete structured payload.
  The old wrapper reconstructed only recipient name, code, and expiry and
  discarded `challenge_id`, so reset-payload validation returned a plain
  application error and rolled back eligible administrative and customer
  forgot-password transactions as `500 internal_error`.
- Added database-free validation of complete recovery payloads for both
  `PASSWORD_RESET_OTP` and `CUSTOMER_PASSWORD_RESET_OTP`.
- No API schema, database migration, stored data, secret, or SMTP transport
  behavior changed. Revision `d0059fe` was built and rehosted on the
  development VPS after the focused and repository-wide Go checks passed.

Verification:

- Focused mail, administrative-auth, and customer-auth recovery payload tests,
  documentation verification, route/OpenAPI parity, `go test ./...`,
  `go vet ./...`, and `git diff --check` passed.
- No disposable `TEST_DATABASE_URL` was configured, so the changed recovery
  lifecycle was not executed against PostgreSQL in this slice.
- The running binary identifies `d0059fe`; systemd is enabled and active with
  zero restarts, the loopback-only listener, loopback/public liveness and
  readiness, Swagger UI, and the 70-operation OpenAPI all passed. No recent
  startup error or panic was recorded. No live recovery email was sent during
  deployment verification.

### Safe HTTP request diagnostics deployed to the development VPS

- Built revision `d27e599`, confirmed it has no migration, dependency, Caddy,
  or systemd change, and rehosted `evcmsnew-dev.service` through the shared
  handler with the configured `LOG_LEVEL=DEBUG` environment.
- Preserved the previously installed binary as
  `builds/evcmsnew.pre-d27e599`; no database migration or data mutation was
  required for this logging-only release.

Verification:

- Focused configuration, middleware, and route tests, `go test ./...`,
  `go vet ./...`, formatting checks, and `git diff --check` passed before
  activation.
- The installed binary identifies revision `d27e599`; systemd is enabled and
  active with zero restarts and listens only on `127.0.0.1:18080`.
- Loopback and public liveness/readiness passed, Swagger UI and the live
  70-operation OpenAPI returned `200`, and the journal emitted correlated
  `http_request_started` and `http_request_completed` JSON records with no
  startup error or panic.

### Field-specific CPO conflicts deployed to the development VPS

- Built revision `9760523`, confirmed the release has no migration,
  dependency, environment, Caddy, or systemd changes, and rehosted
  `evcmsnew-dev.service` through the shared handler.
- Created the validated mode-0600 rollback dump
  `/tmp/devevcmsnewdb-pre-9760523.dump` and preserved the previous binary as
  `builds/evcmsnew.pre-9760523`.
- Ran the forward migrator as a no-op at migration eleven and preserved all
  four complete CPO records.

Verification:

- Focused conflict mapping and runtime/OpenAPI tests, documentation contract
  verification, `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- The installed binary matches the clean `9760523` candidate. Systemd,
  loopback/public readiness, the live 70-operation OpenAPI and Swagger UI,
  protected routes, retired-route absence, required workers, and the startup
  journal passed.
- The live OpenAPI advertises the field-specific slug and GSTIN conflict codes.
  No live CPO mutation or mail delivery was used for deployment verification.

### Safe structured HTTP request logging implemented

- Added one always-on newline-delimited JSON completion record for every
  request that reaches Gin, including request ID, method, matched route
  template, status, latency, response size, direct/effective client address,
  and log level.
- Added server-generated `X-Request-ID` to every Gin response and exposed it
  through the existing permissive CORS middleware for frontend support
  correlation. Client-supplied IDs are not adopted.
- Enriched successfully authenticated requests with only trusted opaque
  scope/user/CPO/customer/role identifiers and handled failures with their
  stable API `error_code`.
- Installed logging outside custom panic recovery so recovered `500` requests
  are recorded. Replaced Gin's debug-mode request dump with a correlated JSON
  stack diagnostic that excludes the panic value and all request content.
- Trusted `X-Forwarded-For` only from a loopback peer for the documented Caddy
  topology; direct callers cannot use that header to spoof `client_ip`.
- Explicitly excluded bodies, raw paths, query strings, headers, emails, user
  agents, app IDs, credentials, tokens, OTPs, API messages, database errors,
  and panic values.
- Added focused middleware/route/leak tests, the internal logging contract, ADR
  0011, OpenAPI/global HTTP guidance, FE correlation guidance, operations
  instructions, explicit developer field/severity/correlation guidance,
  plan/state updates, and documentation drift checks.
- Added validated `LOG_LEVEL=INFO|DEBUG` configuration. DEBUG adds a safe
  request-start event and a handled-error event containing only request ID,
  component, status, stable error code, Go error type, safe class, and optional
  PostgreSQL SQL state; it does not relax any data exclusion.

Verification at the time of this entry:

- Focused request logging, handled-error correlation, recovered-panic stack and
  request-dump exclusion, INFO/DEBUG mode behavior, safe error classification,
  authentication failure, and CORS exposure tests passed.
- Documentation verification, the OpenAPI/runtime/Swagger route test,
  `go test ./...`, `go vet ./...`, and `git diff --check` passed after the
  DEBUG-mode extension.
- The source slice was committed and pushed through `anubhab-work` before its
  integration into `main`; no deployment or remote service mutation was
  performed.

### SuperAdmin CPO conflicts made field-specific

- Traced the live generic `409 cpo_conflict` to PostgreSQL constraint names;
  the latest observed failed creation was rejected by normalized GSTIN
  uniqueness, while earlier attempts included normalized slug collisions.
- Replaced the generic mapping for known CPO writes with stable
  `cpo_slug_conflict`, `cpo_gstin_conflict`, `cpo_app_id_conflict`,
  `admin_identity_conflict`, `cpo_admin_membership_conflict`, and
  `cpo_primary_admin_conflict` codes and actionable messages.
- Retained HTTP `409`, the existing error envelope, and `cpo_conflict` as the
  forward-compatible fallback for an unknown unique constraint.
- Updated focused tests, PostgreSQL lifecycle expectations, OpenAPI, the human
  API contract, CPO administration guide, SuperAdmin FE handoff, development
  plan, active control-plane plan, and project state.

Verification at the time of this entry:

- Constraint-mapping and affected CPO validation tests passed.
- Documentation verification, focused runtime/OpenAPI/Swagger parity,
  `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- Updated PostgreSQL lifecycle assertions compiled but did not execute because
  no explicitly disposable `TEST_DATABASE_URL` is configured.

### Required CPO registration release deployed to the development VPS

- Confirmed `main` was already at remote revision `afd90f5`, reviewed migration
  eleven and the 70-operation contract, and verified all three existing CPOs
  already satisfied the new GSTIN/address precondition.
- Created the validated mode-0600 rollback dump
  `/tmp/devevcmsnewdb-pre-afd90f5.dump` and preserved the previous binary as
  `builds/evcmsnew.pre-afd90f5`.
- Applied migration eleven without changing or removing any CPO row, installed
  the clean candidate, and rehosted `evcmsnew-dev.service` through the shared
  handler.

Verification:

- Documentation verification, focused migration/CPO/auth/customer/integration/
  route tests, runtime/OpenAPI/Swagger parity, `go test ./...`, `go vet ./...`,
  and `git diff --check` passed before activation.
- The migration ledger records version eleven; GSTIN is non-null, all four
  address checks are active, and no incomplete CPO exists.
- The running binary matches the `afd90f5` candidate. Systemd is enabled and
  active with zero restarts; loopback/public readiness, live 70-operation
  OpenAPI, protected slug availability, retired-route absence, required-worker
  freshness, and the post-start journal passed.
- The disposable PostgreSQL lifecycle was not run because deleting a test
  database was not authorized. No live CPO mutation or mail delivery was used
  to exercise the new contract during deployment.

### Required CPO registration identity and slug availability implemented

- Made GSTIN, address, city, state, and pincode mandatory for platform CPO
  creation and full profile replacement, with field-specific validation and
  always-present response fields.
- Added migration eleven to make GSTIN non-null, remove empty address defaults,
  and enforce nonblank address fields. The migration fails closed on incomplete
  existing CPO rows instead of fabricating tenant legal/address data.
- Preserved the existing normalized unique indexes as the authoritative
  case-insensitive slug and GSTIN collision guards.
- Added platform-authenticated
  `GET /api/v1/platform/cpos/slug-availability?slug=...`, returning the
  normalized slug and an advisory availability snapshot. Creation still
  handles the concurrency race with `409 cpo_conflict`.
- Updated Go models, validation, route protection, PostgreSQL lifecycle
  coverage, OpenAPI, the human API contract, SuperAdmin FE handoff, CPO guide,
  schema record, plans, project state, and ADR 0010.

Verification at the time of this entry:

- Changed packages, migration discovery/content, validation, authorization,
  and compile-time PostgreSQL lifecycle coverage passed.
- Documentation verification, the focused runtime/OpenAPI/Swagger parity test,
  `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- The disposable PostgreSQL path did not execute because `TEST_DATABASE_URL`
  is not configured; no migration was applied to the live development database.
- No commit, push, deployment, mail delivery, or remote database mutation was
  performed.

### Recovery-mail correction deployed to the development VPS

- Fast-forward checked `main` and confirmed revision `1cec3f3` was already the
  current remote head.
- Confirmed the release changes no migration, dependency, or deployment
  configuration and that no legacy credential-bearing mail job was pending or
  processing before activation.
- Created the validated mode-0600 rollback dump
  `/tmp/devevcmsnewdb-pre-1cec3f3.dump`, preserved the previous binary as
  `builds/evcmsnew.pre-1cec3f3`, confirmed migration ten remained current, and
  rehosted `evcmsnew-dev.service` through the shared handler.

Verification:

- Documentation drift checks, focused auth/customer/mail/CPO tests,
  runtime/OpenAPI/Swagger parity, `go test ./...`, `go vet ./...`, and
  `git diff --check` passed before activation.
- The running binary hash and module metadata match the clean `1cec3f3`
  candidate. Systemd is enabled and active with zero restarts.
- Loopback and public liveness/readiness, Swagger UI, the live 69-operation
  OpenAPI document, protected-route rejection, retired-route absence, required
  worker freshness, migration ledger, and post-start journal passed.
- No live recovery/onboarding email or authenticated password-reset mutation
  was triggered during deployment. The changed PostgreSQL lifecycle remains
  unexecuted because dropping a disposable database was not authorized.

### Recovery IDs and first-admin credential delivery corrected

- Added the opaque authentication challenge ID to encrypted administrative and
  customer password-reset mail payloads and rendered it beside the six-digit
  code and expiry. Forgot-password responses remain unchanged and generic, so
  the correction does not create an account-enumeration signal.
- Updated administrative and customer lifecycle coverage to obtain both reset
  inputs from the recipient-visible encrypted mail payload rather than reading
  challenge IDs from internal database state.
- Made credential-bearing mail fail closed before enqueue and again before SMTP
  rendering: reset mail requires a parseable recovery ID/code/expiry, and
  `CPO_ADMIN_WELCOME` requires the generated temporary password.
- Added renderer tests proving both reset templates contain recovery ID/code/
  expiry and the new-identity CPO welcome contains its temporary password and
  CPO/app identifiers.
- Preserved global-identity ownership: only `identity_created=true` receives a
  temporary password. Reused active identities keep their existing password,
  and onboarding resend remains credential-free with working recovery guidance.
- Corrected SuperAdmin/CPO/customer/OpenAPI/mail documentation to distinguish a
  committed outbox job from confirmed `SENT` delivery and to mark recovery as
  frontend-completable for newly generated emails.

Verification:

- Focused mail, administrative-auth, customer-auth, and CPO package tests
  passed.
- Documentation drift and focused runtime/OpenAPI/Swagger parity checks passed.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- Updated PostgreSQL lifecycle tests compiled but did not execute because no
  explicitly disposable `TEST_DATABASE_URL` was configured.
- Real SMTP delivery, authenticated live recovery/onboarding, database mutation,
  commit, push, and deployment were not performed.

### Exhaustive SuperAdmin frontend integration handoff added

- Added `docs/SUPERADMIN_FRONTEND_HANDOFF.md` as the canonical no-chat-history
  handoff for the platform SuperAdmin frontend.
- Covered the 27-operation SuperAdmin integration surface, environment and
  authorization boundary, screen model, TypeScript request/response types,
  login/OTP/refresh/session state machine, complete CPO workflows,
  audit/workers, authenticated fetch-based SSE, REST replay/cursor recovery,
  event invalidation, error UX, mutation retry rules, security, testing, and
  definition of done.
- Explicitly separated callable behavior from planned platform governance,
  security, generic mail, announcement, and overview surfaces; retained manual
  CPO access and the prohibition on tenant-data/secret access and
  subscriptions/platform billing.
- Registered the handoff in repository instructions, root/documentation
  indexes, project state, development planning, the active SuperAdmin plan,
  and documentation drift verification.
- Reconciled realtime guidance with actual producers: profile-update events
  carry an empty data object, app-ID events carry only the mode, and the
  OpenAPI SSE example now uses an emitted CPO event. Corrected optional-field
  examples and the replay section reference in the human API contract.
- Found that administrative and customer forgot-password create recovery
  challenges but neither their response nor current email delivers the
  challenge ID required by reset/resend. Documented both frontend gaps across
  OpenAPI, human/workflow contracts, state, and plans; changed the affected
  feature/plan statuses from `Verified` to `Implemented`. No authentication
  behavior was changed in this documentation-only slice.

Verification:

- Public development liveness/readiness, Swagger UI, and the deployed
  69-operation OpenAPI document returned successfully before editing.
- Documentation registration/drift verification passed.
- Focused runtime/OpenAPI parity and Swagger-serving verification passed.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- No authenticated deployment workflow, database mutation, mail delivery, or
  deployment was performed.

## 2026-07-31

### Canonical CPO backend AI-agent handoff added

- Added `docs/CPO_BACKEND_AGENT_HANDOFF.md` as the no-chat-history orientation
  and execution guide for agents assigned to CPO-side backend work.
- Recorded the current ADMIN-only and CUSTOMER plane split, implemented route
  inventory, actual code/data ownership, schema-versus-behavior warning,
  CMS/HAL boundary, exact transaction-ID rule, and explicit unimplemented
  surfaces.
- Added dependency-aware customer, network, tariff, HAL, charging, settlement,
  reporting, realtime, and deferred staff-management guidance without marking
  any unassigned slice approved.
- Included mandatory tenant-isolation, endpoint, migration, verification, Git,
  documentation, definition-of-done, and final-handoff checklists.
- Registered the guide in repository agent instructions, documentation
  navigation, development planning, project state, and documentation drift
  verification.

Verification:

- Documentation registration and mandatory handoff-rule checks passed.
- Runtime/OpenAPI parity, `go test ./...`, `go vet ./...`, and
  `git diff --check` passed.
- This documentation-only slice did not mutate or exercise a database, SMTP,
  HAL, or deployed service.

### CPO administrator network release verified and deployed

- Reviewed revision `407ec07` and confirmed that its 20 new ADMIN-only CPO
  operations require no database migration beyond the existing migration ten.
- Ran the complete CPO organization/profile/network/pricing lifecycle against
  an explicitly disposable PostgreSQL database. That verification exposed and
  corrected the `open_24_hours` and `price_per_kwh` GORM column mappings and
  PostgreSQL `23001` dependency-violation handling for `charger_in_use`.
- Added focused regression coverage for the migration-aligned column mappings
  and both PostgreSQL dependency-violation codes.
- Created the validated mode-0600 rollback dump
  `/tmp/devevcmsnewdb-pre-407ec07.dump`, preserved the previous binary as
  `builds/evcmsnew.pre-407ec07`, confirmed migration ten remained current, and
  rehosted `evcmsnew-dev.service` through the shared handler.

Verification:

- The disposable PostgreSQL lifecycle, focused model/error tests, route/OpenAPI
  parity, documentation verification, `go test ./...`, `go vet ./...`, and
  `git diff --check` passed.
- The installed binary hash matches the separately built candidate based on
  revision `407ec07` plus the local compatibility corrections. Those
  corrections and this deployment record are published together in this
  changeset.
- Systemd is enabled and active with zero restarts. Loopback and public
  liveness/readiness, Swagger UI, raw OpenAPI, the live 69-operation contract,
  new-route authentication rejection, retired-route absence, and both required
  logical workers passed.
- All ten migrations remain recorded, with 31 public runtime tables and all 11
  retired commercial tables preserved. The development service emits Gin's
  expected debug-mode warning at startup and no application error or panic.

### CPO organization registration details exposed read-only

- Added `GET /api/v1/cpo/organization` for the current CPO ADMIN to read the
  organization data registered and managed by Superadmin.
- Derived the CPO exclusively from the verified session and retained the
  existing bearer, current app-ID, active-membership, and ADMIN-only boundary.
- Returned business/registration fields, lifecycle status/timestamp, current
  app ID/mode, and record timestamps while omitting the internal platform actor
  and privileged lifecycle reason.
- Kept organization mutation exclusively on the platform Superadmin endpoint;
  the tenant route is side-effect-free and emits no audit event.
- Expanded the authoritative runtime/OpenAPI surface from 68 to 69 operations
  and updated the human contract, workflow guide, plan, ADR, project state,
  route verification, and PostgreSQL lifecycle verifier.

Verification:

- Tenant-safe projection and protected-route tests passed, including explicit
  checks that privileged lifecycle fields are absent.
- Documentation verification and bidirectional runtime/OpenAPI parity passed
  at 69 operations; `go test ./...`, `go vet ./...`, and `git diff --check`
  passed.
- The PostgreSQL lifecycle verifier, including the organization read, passed
  against an explicitly created and removed disposable database.

### Contributed CPO work reconciled into an ADMIN-only tenant surface

- Merged the divergent `abhranil_ev_cms_backend_new` history into the
  authoritative line without rewriting or deleting the contributor branch.
- Preserved the verified platform CPO control-plane implementation while
  removing the contributed tenant-side CPO organization profile.
- Closed CPO authentication, integrations, and tenant business operations to
  the one currently provisioned `ADMIN` authority. Kept OWNER, OPERATOR, and
  VIEWER only as dormant future-compatible persisted enum values.
- Added CPO administrator identity-profile read/update for global full name and
  phone, keeping `/api/v1/auth/me` as authentication bootstrap and keeping
  organization fields platform-owned.
- Corrected and integrated tenant-scoped hub, charger/connector, GST, and tariff
  operations with server-generated IDs, cross-tenant related-record checks,
  exact decimals, INR defaulting, connector length validation, bounded
  hub/charger/GST/tariff pagination, transactional audit evidence, and
  dependency-safe charger deletion.
- Kept CMS network inventory separate from the HAL. Generated
  `ocpp_identity` is a mapping value only; no handshake, protocol state, live
  status, command, callback, or reconciliation behavior was added.
- Expanded the source OpenAPI/runtime surface from 49 to 68 operations and
  added the complete human contract, workflow guide, HAL boundary update,
  implementation plan, and ADR 0009.

Verification:

- Focused auth, integration-credential, CPO, protected-route,
  absent-organization-profile, and runtime/OpenAPI tests passed.
- Documentation verification passed at 68 operations; `go test ./...`,
  `go vet ./...`, residue scanning, and `git diff --check` passed.
- The PostgreSQL end-to-end verifier passed against an explicitly disposable
  database. The source feature is therefore `Verified`.
- The development VPS now serves the 69-operation candidate based on revision
  `407ec07` plus the local PostgreSQL compatibility corrections.

### CPO dependency release deployed to the development VPS

- Reviewed source revision `91cc5ba`, the additive migration-ten backfill, the
  49-operation HTTP contract, and the existing Caddy/systemd deployment.
- Confirmed before migration that the development database contained no CPO or
  membership rows and no pending or processing mail jobs.
- Created the mode-0600 pre-migration PostgreSQL backup
  `/tmp/devevcmsnewdb-pre-91cc5ba.dump` and preserved the previous binary as
  `builds/evcmsnew.pre-91cc5ba`.
- Applied migration ten, installed the separately verified candidate binary,
  and rehosted `evcmsnew-dev.service` through the shared handler.

Verification:

- Documentation verification, focused runtime/OpenAPI parity, `go test ./...`,
  `go vet ./...`, `git diff --check`, and the release build passed.
- All ten migrations are recorded; 31 runtime tables remain in `public` and
  the 11 retired commercial tables remain preserved in `retired_commercial`.
- The running process hash matches the `91cc5ba` candidate, systemd is enabled
  and active with zero restarts, and the post-rehost journal has no warning or
  error entries.
- Loopback and public liveness/readiness, Swagger UI, raw OpenAPI, the live
  49-operation count, new protected-route rejection, required worker freshness,
  and retired-route absence passed.

### CPO dependency on Superadmin completed locally

- Added migration ten for durable CPO lifecycle evidence, one primary
  administrator designation, correlated onboarding-mail metadata, and the
  credential-free resend template.
- Added bounded CPO search/filter/cursor listing, mutable business-profile
  replacement, and reason-required activation/suspension.
- Added safe primary-admin visibility, transactional replacement/restoration,
  onboarding resend, and explicit CPO administrative-session revocation.
- Preserved global identity passwords during reuse, revoked only the replaced
  primary's selected-CPO sessions, and kept platform/customer scopes isolated.
- Emitted committed audit records and durable platform events from the owning
  transactions; no event or API exposes credentials or encrypted mail bodies.
- Expanded the authoritative OpenAPI and human/FE contracts from 44 to 49
  operations and documented exact frontend refresh/recovery behavior.

Verification:

- Migration ten completed an up/down lifecycle against disposable
  loopback-only PostgreSQL 17.
- PostgreSQL lifecycle tests passed CPO creation/mail correlation,
  primary-admin uniqueness, searchable/cursor listing, profile replacement,
  reasoned idempotent lifecycle control, administrator replacement, targeted
  session/refresh revocation, credential-free resend, and platform-session
  isolation.
- Focused CPO, route/OpenAPI, mail, model, and migration tests passed.
- Documentation verification, `go test ./...`, `go vet ./...`,
  `git diff --check`, and final status review passed.

Compatibility and deployment:

- Activation and suspension now require `{"reason":"..."}`; this deliberate
  pre-frontend contract change is documented in OpenAPI.
- Existing CPO IDs, slugs, app IDs, auth scopes, and tenant boundaries remain
  unchanged. Existing CPOs are backfilled deterministically where an eligible
  administrator exists.
- The verified source slice was published to `main`. It was not deployed; the
  development VPS remains on migrations one through nine and its deployed
  44-operation contract.

## 2026-07-24

### Commercial retirement deployed and worker readiness corrected

- Fast-forwarded the clean `main` checkout from `3b01a5d` to `b947ae1`.
- Confirmed all eleven retiring commercial prototype tables and related pending
  mail queues were empty before migration.
- Created the mode-0600 pre-migration backup
  `/tmp/devevcmsnewdb-pre-b947ae1.dump`.
- Applied migration nine, preserving the eleven tables under
  `retired_commercial` and disabling the subscription-lifecycle and
  billing-maintenance worker records.
- Rebuilt and rehosted the 44-operation manual-CPO-access release.
- Diagnosed pre-existing readiness oscillation caused by a `30s` worker stale
  threshold with a `1m` maintenance heartbeat and by stale required instance
  rows left by replaced processes.
- Changed the default and deployment stale threshold to `2m`, required it to
  exceed the maintenance interval, and made readiness require at least one
  fresh healthy instance per required worker name.

Verification:

- Deployment configuration, focused route/OpenAPI verification, `go test
  ./...`, `go vet ./...`, documentation verification, and `git diff --check`
  passed.
- The worker replacement behavior passed against an explicitly created and
  immediately removed disposable PostgreSQL database.
- All nine migrations are recorded; 31 runtime tables remain in `public` and
  11 archived tables exist in `retired_commercial`.
- Retired commercial routes return `404`; the retained worker route returns
  `401` without authentication.
- Loopback and public liveness/readiness, Swagger UI, running-binary identity,
  and zero error-level service logs passed.
- Public readiness remains healthy while stale historical platform-maintenance
  and mail-outbox rows coexist with fresh replacement instances.

### Tenant commercial management retired

- Replaced the tenant subscription, entitlement, platform-invoice, and
  platform-payment direction with manual superadmin CPO activation/suspension.
- Removed the subscription and platform-billing runtime modules, routes,
  models, workers, mail producers, OpenAPI operations, and active contracts.
- Added migration nine, which preserves the already-deployed prototype tables
  in `retired_commercial` rather than dropping data.
- Migration nine disables retired worker requirements and refuses to proceed
  while a related mail job is pending or processing.
- Added ADR 0008 and rewrote the superadmin plan, concepts, API handoff, schema
  ownership, and project state around the simpler access model.

Verification:

- Migration nine passed disposable PostgreSQL 17 up, pending-mail guard,
  preserved-row archival, worker disablement, down restoration, and worker
  re-enable checks.
- Retired routes return `404`; all 44 runtime/OpenAPI operations match.
- Focused tests, full `go test ./...`, `go vet ./...`, documentation
  verification, residue scans, and `git diff --check` passed.

### Development VPS updated to the control-plane release

- Fast-forwarded the clean `main` checkout from `d610efe` to `e15699d`.
- Added the new non-secret API documentation and platform worker settings to
  the ignored deployment environment while preserving its existing secrets.
- Built and checked a replacement binary before changing the running service.
- Created a mode-0600 pre-migration PostgreSQL custom-format backup at
  `/tmp/devevcmsnewdb-pre-e15699d.dump`.
- Applied forward migrations six through eight, bringing the migration ledger
  to eight versions and the development schema to 42 public tables.
- Preserved the previous binary as `builds/evcmsnew.d610efe`, installed the new
  binary, and invoked the shared `rehost-service` handler.

Verification:

- Deployment configuration loading, focused runtime/OpenAPI verification,
  `go test ./...`, `go vet ./...`, the Bash-equivalent documentation contract
  check, and `git diff --check` passed.
- The service is enabled and active with zero restarts, and systemd is running
  the exact newly built binary.
- Loopback and public liveness/readiness, Swagger UI, and raw OpenAPI passed.
- The new platform worker API rejects unauthenticated access with `401`.
- Platform maintenance, subscription lifecycle, billing maintenance, and mail
  outbox workers registered healthy.
- All representative platform operations, subscription, and billing tables
  exist, and the post-rehost journal contains no error-level entries.
- Granular PostgreSQL lifecycle integration tests remain pending because this
  deployment database is not an authorized disposable test target.

### Complete superadmin control plane approved and started

- Inventoried the implemented new-CMS platform, auth, CPO, mail, audit, and
  documentation surfaces.
- Inspected the legacy CMS data model and route behavior instead of treating
  its mixed admin route names as authoritative.
- Confirmed the legacy CMS has no durable subscription model to preserve.
- Approved a provider-neutral subscription and entitlement model in which CPO
  creation and lifecycle remain independent from subscription state.
- Approved complete platform operations for CPO/admin recovery, platform
  administrators, subscription billing records, audit/security, mail,
  notifications, workers, announcements, overview, system status, and durable
  SSE realtime with REST replay.
- Recorded the binding implementation plan and ADR 0007.
- Added the required `API_DOCS_ENABLED` configuration toggle for Swagger UI and
  raw OpenAPI route registration while preserving the enabled compatibility
  default.

Implementation:

- Focused configuration and route tests passed for documentation enabled and
  disabled behavior.
- Implemented durable platform events, audit queries, worker heartbeats,
  readiness degradation, REST replay, and authenticated SSE.
- Added versioned subscription plans, database-immutable published snapshots,
  exact pricing, entitlements, optional CPO assignments, complete lifecycle
  transitions, scheduled boundary changes, idempotent history, overrides,
  CPO-admin mail, audit, events, and lifecycle reconciliation worker.
- Added provider-neutral platform billing accounts, immutable issued invoice
  terms/lines, exact arithmetic, payment allocation and reversal, timeline
  queries, overdue reconciliation, invoice mail, audit, and durable events.
- Expanded OpenAPI to all 74 runtime operations and added granular realtime,
  subscription, and platform-billing contracts.

Verification:

- Focused Go tests, OpenAPI semantic validation, route/OpenAPI parity, route
  protection, model parsing, migration discovery, mail tests, and documentation
  verification passed.
- Full `go test ./...`, `go vet ./...`, documentation verification, and
  `git diff --check` passed after the billing slice.
- Disposable PostgreSQL execution for migrations six through eight and their
  lifecycle tests remains pending.

## 2026-07-23

### Development VPS hosting prepared and activated

- Prepared `dev-evcmsnew.transev.site` behind Caddy with an application
  listener reserved on `127.0.0.1:18080`.
- Created the additive PostgreSQL database `devevcmsnewdb`, owned by
  `postgres`.
- Created an ignored mode-0600 `.env` from `.env.example`, set the requested
  initial superadmin identity, and generated five independent cryptographic
  keys.
- Added `evcmsnew-dev.service`, centralized ignored `builds/` output, and the
  `rehost-evcmsnew` interactive handler.
- Added the supplied database and SMTP passwords only to the ignored mode-0600
  deployment environment.
- Applied all five forward migrations, bootstrapped the configured platform
  superadmin, and enabled and started the application service.
- Added the authoritative development-hosting and activation guide.

Verification:

- Caddy configuration validation and reload completed.
- The database owner, loopback listener, DNS resolution, binary build,
  documentation checks, Go tests, and Go vet were verified.
- PowerShell is not installed on this VPS, so `scripts/verify-docs.ps1` could
  not run directly; its required-file, route-presence, and removed-setting
  checks were reproduced in Bash and passed.
- The migration ledger contains all five versions and 29 public tables.
- The service is enabled and active; loopback and public HTTPS liveness and
  database-readiness checks passed.
- A platform-login request returned `202`, confirmed the bootstrapped
  superadmin, and produced a real `LOGIN_OTP` delivery that reached `SENT` on
  the first attempt.

### Temporary remote-development access implemented

- Added environment-controlled permissive CORS middleware with preflight
  handling for remote browser frontends.
- Set the current ignored `.env` and checked-in `.env.example` to listen on
  `0.0.0.0:8080` with `CORS_ALLOW_ALL=true`.
- Kept the code defaults at loopback-only and CORS-disabled when those
  variables are absent.
- Preserved authentication, CPO app-ID validation, tenant isolation, and
  authorization for all actual API requests.
- Documented the remote client URL, operational risk, and production rollback.

Verification:

- Configuration and route tests cover enabled and disabled CORS behavior.
- Documentation verification, runtime/OpenAPI route matching, `go test ./...`,
  and `go vet ./...` passed.

### Complete app-user authentication boundary implemented and verified

- Approved a distinct `CUSTOMER` session scope tied to one CPO customer.
- Defined app password-plus-mail-OTP login, encrypted access tokens, rotating
  refresh tokens, `me`, scoped session management, logout, and password
  operations.
- Defined trusted backend helpers for current user, customer, CPO, and app
  identity.
- Kept customer session operations isolated from administrative and other-CPO
  sessions while retaining global password revocation semantics.
- Added the `000005_customer_authentication` migration with a database-enforced
  user/customer/CPO session identity and customer challenge/mail contracts.
- Added customer login verify/resend, refresh, `me`, session
  list/revoke/logout, password recovery/reset/resend/change, and protected app
  middleware.
- Added trusted `customerauth.CurrentPrincipal`, `CurrentUserID`,
  `CurrentCustomerID`, `CurrentCPOID`, and `CurrentCPOAppID` helpers.
- Expanded the human API contract and canonical OpenAPI to all 40 operations.

Verification:

- Migration down, up, and idempotent-up passed in PostgreSQL 17.
- Customer authentication lifecycle passed for login OTP, encrypted access,
  `me`, refresh rotation/reuse, session isolation/revocation, recovery/change,
  and global password session revocation.
- Documentation verification and all 40 runtime/OpenAPI operation matches
  passed.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.

### CPO-scoped customer signup implemented and verified

- Approved public email-verified signup under an active CPO app identity.
- Kept `X-CPO-App-ID` as public routing metadata rather than authentication.
- Defined durable pending challenges that retain only a password hash and
  HMAC-protected OTP.
- Defined safe global identity reuse without password or profile overwrite.
- Defined transactional customer and zero-balance INR wallet creation without
  a subscription dependency.
- Kept customer login and session issuance outside this slice.
- Added the `000004_customer_signup` up/down migration, CPO-bound challenge
  model, public start/verify/resend handlers, mail template, audit write, and
  transactional customer/wallet creation.
- Added explicit human and OpenAPI contracts plus runtime/spec drift coverage.

Verification:

- PostgreSQL migration down, up, and idempotent-up passed.
- PostgreSQL signup lifecycle passed for resend invalidation, replay rejection,
  global identity creation/reuse, password/profile preservation, CPO isolation,
  and zero-balance INR wallet creation.
- Documentation verification and all 27 runtime/OpenAPI operation matches
  passed.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.

### Hostinger SMTP contract and durable documentation surfaces

- Replaced the string SMTP mode with mutually exclusive
  `SMTP_USE_SSL`/`SMTP_USE_TLS` flags.
- Configured the checked-in example for `team@transev.in` through
  `smtp.hostinger.com:465` using implicit TLS, while keeping the password
  environment-only.
- Rejected plaintext, conflicting transport modes, and half-configured SMTP
  credentials during startup validation.
- Added focused Hostinger configuration and SMTP-construction tests.
- Added a canonical documentation map, educational identity/onboarding/support
  guides, external integration records, and API/internal/configuration
  contracts.
- Registered the new documentation surfaces in repository instructions and
  added a PowerShell documentation verifier.
- Expanded the human API contract to document every implemented endpoint,
  request/response, authorization rule, validation, state effect, and error.
- Added canonical OpenAPI 3.1, embedded same-origin Swagger UI, raw spec
  serving, semantic validation, and bidirectional runtime-route drift tests.
- Expanded educational, SMTP, Razorpay, HAL, mail-outbox, and configuration
  documentation to integration-handoff detail.

Verification:

- Focused configuration and mail tests passed.
- Documentation contract and removed-configuration residue checks passed.
- OpenAPI parsed and passed semantic validation.
- All 24 implemented business/health operations matched the runtime router and
  OpenAPI in both directions.
- Embedded Swagger UI, redirect, and raw-spec endpoints passed HTTP smoke tests.
- `go test ./...`, `go vet ./...`, and `git diff --check` passed.
- Real Hostinger delivery was not attempted and remains an operational check.

### CPO provisioning and app identity verified

- Approved CPO creation without a subscription dependency.
- Defined a server-generated dummy app ID at creation and a separate
  superadmin-controlled live app-ID transition.
- Kept lifecycle status independent from dummy/live app-ID status so onboarding
  can run with an active CPO and dummy ID.
- Defined `X-CPO-App-ID` as tenant-routing metadata that must match the
  authenticated CPO but never replaces user/session authorization.
- Exempted health, authentication, platform control-plane, and future
  independently authenticated callback surfaces from the tenant app-ID header.
- Expanded creation to atomically establish the first CPO admin.
- Defined encrypted delivery of a generated temporary password for a new
  identity and safe existing-identity reuse without password overwrite.
- Defined durable first-login password-change enforcement with no expiry and a
  reminder after each successful login until the password changes.

Verification:

- Architecture and implementation plan recorded.
- Added the `000003_cpo_provisioning` up/down migration and embedded migration
  coverage.
- Added platform CPO create/list/get/activate/suspend/live-app-ID APIs.
- Added atomic first-admin membership, encrypted onboarding or assignment
  email, non-expiring temporary-password change enforcement, and login
  reminders.
- Serialized concurrent first-admin identity creation per normalized email.
- Added current app identity to CPO login, refresh, and `me`, plus
  `X-CPO-App-ID` enforcement on tenant business APIs.
- Added session and refresh-token revocation on CPO suspension.
- Focused and PostgreSQL lifecycle tests passed.
- Migration down, up, and idempotent up passed on PostgreSQL 17.
- Full PostgreSQL-backed `go test ./...`, `go vet ./...`, residue scanning, and
  `git diff --check` passed.

### Authentication and credential boundary started

- Approved one shared credential boundary for platform superadmins and CPO
  staff while preserving separate authorization scopes.
- Defined an idempotent environment-only first-superadmin bootstrap that never
  overwrites an existing administrator password.
- Selected mandatory email OTP for administrative login, a durable PostgreSQL
  mail outbox worker, encrypted access tokens, rotating opaque refresh tokens,
  session revocation, password recovery, and trusted tenant helpers.
- Defined CPO-admin write-only encrypted Razorpay credentials without
  superadmin plaintext access.
- Kept public signup, social login, SMS/TOTP/passkeys, custom RBAC, Redis,
  external queues, and payment execution outside this slice.

### Authentication and credential boundary verified

- Added the additive `000002_auth_credentials` up/down migration for user
  security state, challenges, sessions, refresh lineage, encrypted mail jobs,
  durable rate limits, and tenant integration credentials.
- Added Argon2id password handling, independent OTP/secret encryption, and
  fixed-algorithm signed-then-encrypted access JWTs.
- Added concurrency-safe idempotent environment bootstrap without existing
  password overwrite.
- Added platform/CPO password-plus-email-OTP login, resend, refresh rotation,
  refresh-reuse revocation, identity/session APIs, password recovery/change,
  and authorization helpers.
- Added a retrying PostgreSQL outbox worker with mandatory SMTP TLS.
- Added CPO owner/admin Razorpay credential create/rotate, metadata, delete, and
  internal resolve behavior. APIs never return secret plaintext.
- Added complete API/configuration documentation and focused plus PostgreSQL
  integration tests.

Verification:

- Focused cryptography, auth, worker, integration-secret, configuration, model,
  and route tests passed.
- PostgreSQL 17 migration down, up, and idempotent up passed.
- PostgreSQL authentication, recovery, mail-worker, and credential-isolation
  integration tests passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- Real SMTP-provider delivery was not run because no provider credentials were
  supplied.

### Lean CMS foundation started

- Replaced the empty scaffold concept with a documented multi-tenant CPO CMS
  boundary.
- Kept the OCPP HAL explicitly separate from the CMS and deferred the handshake
  contract to the relevant implementation phase.
- Chose fixed CPO-wide roles instead of custom RBAC or per-hub scopes.
- Defined the initial platform-admin, user, CPO membership, and customer data
  model.
- Added versioned migration, database startup, and health-service foundations.
- Recorded custom roles, plan entitlements, RLS, event infrastructure, charging,
  and finance as deferred work rather than speculative implementation.

Verification:

- Go formatting completed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- PostgreSQL migration integration remains unverified because no explicitly
  selected disposable development database was used.

### Complete supplied CMS schema restored

- Corrected the initial over-simplification that omitted most supplied CMS
  domain data.
- Added user settings, tenant user groups, hubs, chargers, connectors, group
  access links, customer favorites, GST profiles, tariffs, wallets, wallet
  transactions, charging sessions, payments, and audit logs.
- Preserved CPO profile data on the CPO tenant and mapped app users to
  tenant-scoped customer records.
- Added CPO identifiers and tenant-matching foreign keys throughout the owned
  domain.
- Added exact-decimal Go types and PostgreSQL numeric fields for finance.
- Added matching `000001_cms_schema.up.sql` and
  `000001_cms_schema.down.sql` migrations.
- Added an explicit latest-migration rollback operation and migration command.
- Added tests for complete table coverage, up/down pairing, JSONB behavior, and
  GORM model parsing.

Verification:

- Focused migration, model, JSONB, and constants tests passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- PostgreSQL 17 up migration created all 21 domain tables.
- A second up run was idempotent and retained one migration version.
- Down migration removed all domain tables and retained only
  `schema_migrations`.
- The loopback-only disposable PostgreSQL instance was stopped and its local
  test directory was removed.
## 2026-08-28 - Support notification delivery

- Completed support notification delivery without changing the core support
  state machine: ticket creation, platform replies, and platform
  resolved/closed/reopened transitions now enqueue encrypted CPO mail intents
  in the same transaction as the authoritative ticket mutation.
- Mail carries only localized status/subject context and the configured support
  action link; it does not include a private message body or require a user to
  handle a ticket UUID outside that link. Active eligible CPO recipients are
  derived/deduplicated from current memberships.
- CPO-created/replied/reopened support activity now uses the existing durable
  platform-event mechanism. No platform support email address was invented.

Verification: focused `src/support` and `src/mail` tests pass. Repository-wide
verification is recorded with this implementation slice; disposable PostgreSQL
coverage remains conditional on `TEST_DATABASE_URL`.
