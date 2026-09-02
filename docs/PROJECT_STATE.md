# Project State

## Current State

### 2026-09-02 - Database-owned trace replay sequence source fix, not deployed

- `ChargingTraceEvent.ingestion_sequence` is now GORM read-only. PostgreSQL's
  migration-000060 default sequence is the sole allocator: trace writers may
  read the stored cursor for CPO replay/SSE but cannot include it in INSERT or
  UPDATE statements. This fixes the observed explicit-zero unique conflict
  without changing charging, fact, session, or trace ordering semantics.
- No migration, database repair, service restart, or deployment was performed.
  The historical zero-valued diagnostic row remains untouched; the healthy
  sequence will naturally allocate its next value without colliding with it.

### 2026-09-02 - Diagnostic trace push pipeline source verified, not deployed

- The current source replaces the deployed query-time HAL trace merge with an
  isolated HAL-to-CMS diagnostic event pipeline. CMS accepts the dedicated
  bearer only at `POST /v1/hal-trace-events`, adopts immutable trace roots,
  and serves CPO static trace and replayable SSE reads entirely from its own
  durable projection. Trace evidence remains non-authoritative.
- The source adds additive migration 000060; it and the matching HAL migration
  019 have not been applied. No service was restarted or deployed. The current
  deployed trace behavior remains the 2026-09-01 baseline below until an
  explicitly authorized coordinated rollout.

### 2026-09-02 - Customer charger fallback correction deployed

- Production evidence showed that the published nested-preload-only customer
  fallback still allowed valid persisted sessions to serialize a zero-value
  charger. The deployed source instead performs one bounded charger lookup
  for all missing rows in the page, scoped to the authenticated CPO and keyed
  first by the persisted connector charger ID, then the materialized session
  charger ID. It is a read projection only and does not write session state.
- Customer history, detail, JSON live snapshots, and both replacement SSE
  frame types share this repair. Runtime revision `48d196f` is active behind
  Caddy with binary SHA-256
  `ceb9e0de19c5c2141926c8f580d84c5d3efbaad6f114bc3aecb2041dc1a5edc1`.
  The stale prior binary is retained at
  `/root/evcmsnew-backups/pre-48d196f-stale-20260902T042542Z`; no database
  mutation was required.

### 2026-09-02 - Charging-session projection coherence deployed

- The deployed source repairs customer charging-session charger hydration:
  the durable direct session charger remains preferred, and only a matching
  same-CPO persisted connector-to-charger relation can supply the historical
  read fallback. Detail/history and live REST/SSE therefore use the same real
  charger UUID, public code, name, and existing customer-visible metadata; no
  serializer invents charger identity.
- CPO live REST snapshots and both full-replacement SSE event types now reuse
  the normal-session static projection for nested customer/charger/connector
  context, first SoC, frozen tariff/tax metadata, start criteria, and CMS
  materialization time, alongside existing live telemetry and unchanged
  financial projection. No migration or database mutation was required. This
  earlier coherence slice was deployed as revision `a1a21a0`; the current
  correction is recorded above.

### 2026-09-01 - CPO historical charging-session commercial projection deployed

- Current source extends CPO charging-session list/detail views with frozen
  `price_per_unit`, optional tariff `unit`, SGST/CGST/IGST percentages, and
  optional customer `start_criteria`/`requested_limit_value` from the durable
  start intent. It does not change price calculation, tariff writes, GST
  writes, charging control, or session authority.
- Historical tariff and tax snapshots win over current mutable tariff/hub GST
  associations. Current entities are a legacy fallback only if the relevant
  session snapshot is absent or unusable. Customer limit selection remains
  independent of tariff billing dimension. No migration or data mutation was
  required. Runtime revision `e263463` is active behind Caddy with binary
  SHA-256 `0f569f47d79f22f5599dbfb2370c29312a5aa42038c5f9ecc53ef4f6ce72bb11`.
  The prior binary is retained at
  `/root/evcmsnew-backups/pre-e263463-20260901T101829Z`.

### 2026-09-01 - Charging transaction trace deployed

- The current CMS source adds additive migration 000059 and a
  CPO-scoped diagnostic waterfall for CMS/HAL charging evidence. CMS creates
  opaque trace IDs before command delivery, retains only sanitized trace
  metadata, exposes `charging_traces.read` to OWNER/ADMIN/OPERATOR defaults,
  and merges private HAL diagnostic evidence without making it authoritative.
- Trace retention is bounded and leaves session, wallet, settlement, command,
  fact, and connector authorities untouched. The source contract has 219
  OpenAPI operations and a CPO frontend handoff. Migration 059 is applied and
  the runtime is active behind Caddy at revision `3037c46` with binary SHA-256
  `2f87378e9bda54d8b8db54877add2c363b8293c7fbd34766a864f9e931759da8`.
  The pre-trace binary is retained at
  `/root/evcmsnew-backups/pre-3037c46-20260901T093716Z`.

### 2026-09-01 - CPO capability authority coherence deployed

- CPO endpoint authority now comes from the documented capability evaluated
  against the active membership and matching app ID. Explicit `DENY` overrides
  role defaults; support, integration, cache-control, session-revocation, and
  OpenAPI security behavior are aligned with that authority. No migration or
  data repair was required.
- Runtime revision `bc1fbe7` was active behind Caddy before the trace release
  with binary SHA-256
  `dbbc77ff75ca46ed95fa1f3a774bbcabcfe3fad52157f386decf39e46100668`.
  Service/readiness, public routing, Swagger/OpenAPI, Caddy, and post-rehost
  startup checks passed. Migration 058 remains applied. The
  prior binary is retained at
  `builds/evcmsnew.pre-deployed-320d489-20260901-095905`.

### 2026-09-01 - CPO capability authority coherence implementation details

- CPO endpoint authority is the documented capability evaluated against the
  current active membership and matching app ID; roles are source-controlled
  default bundles only, and an explicit `DENY` overrides every role default.
  Support reply requires both read and reply authority before mutation;
  integration reads use `settings.read` while mutations use `settings.manage`.
- A real CPO role change transactionally revokes only that user's matching CPO
  administrative sessions and refresh tokens. Override-only edits retain
  sessions. CPO protected responses use `Cache-Control: no-store`, and live
  SSE streams re-evaluate their capability at heartbeat.
- OpenAPI now represents CPO Bearer authentication and `X-CPO-App-ID` as a
  single AND requirement, including hub-GST and user-group membership routes.
  This behavior is included in the active `48d196f` deployment recorded above;
  no migration or data repair was required.

### 2026-08-31 - User App realtime and mail-outbox reconciliation deployed

- The authenticated User App full-state live-session and selected-charger
  availability projections, CPO live-session watermark ordering, and the
  reconciled mail-outbox template catalogue are active in the development
  deployment. Migration 058 is applied with its historical-row-preserving
  `NOT VALID` constraint; no data repair was required.
- Runtime revision `320d489` is active behind Caddy with binary SHA-256
  `37e6397a939ca16b8fb903147b5d7ee80e2f8e1ffa4c21acee448e66af4b413a`.
  Service/readiness, public routing, Swagger/OpenAPI (217 operations), Caddy,
  migration state, and post-rehost startup checks pass. The prior binary and
  pre-migration dump are retained at
  `builds/evcmsnew.pre-deployed-632ec13-20260831-141003` and
  `/root/evcmsnew-backups/devevcmsnew-before-000058-20260831-140835.dump`.

### 2026-08-31 - Mail-outbox template catalogue source correction

- Source now has a single 24-name durable mail-template catalogue shared by
  outbox validation and renderer compatibility checks. Forward migration 058
  replaces the stale `mail_outbox` template CHECK for future writes, including
  the support-ticket lifecycle templates required by transactional status
  notifications.
- The replacement CHECK is `NOT VALID` only so preexisting historical rows are
  preserved rather than deleted or rewritten; PostgreSQL still enforces the
  current catalogue on every new or updated row. Migration 058 is applied on
  the development database; PostgreSQL integration tests remain separately
  gated on an explicitly selected disposable `TEST_DATABASE_URL`.

### 2026-08-31 - User App full-state realtime projections source-verified

- Source now adds authenticated User App full-state SSE for the current
  customer live-session collection and one selected customer-visible charger's
  availability. The legacy retained operational-event REST/SSE feed remains
  available for compatibility but is no longer the state contract for these
  two views.
- Both new streams establish a committed event watermark before reading their
  initial CMS projection, react to durable session/charger/connector wake-up
  facts, and periodically reproject after access revalidation so freshness and
  time-priced projected amounts cannot remain stale without another HAL fact.
  They never synchronously call HAL. A customer can receive multiple concurrent
  materialized sessions, and all emitted updates replace the full current
  projection.
- The existing CPO live-session stream now uses the same watermark-before-
  snapshot safety ordering. This source change is included in the active
  `320d489` deployment recorded above.

### 2026-08-28 - CPO live-session OCPP transaction identity deployed

- CPO live-session snapshots and SSE payloads now expose the durable OCPP
  transaction identifier as `ocpp_transaction_id`, alongside the existing
  CMS session and charger context. The OpenAPI contract and focused projection
  tests match the runtime response. No migration or data repair was required.
- Runtime revision `632ec13d359bafc355d961aa9ff925fa089ac6ac` is active behind
  Caddy with binary SHA-256
  `5595f05de08736f7e7b7e509b9092fe16592e3eda0308f500614b4eb46b33b47`.
   Service/readiness, public routing, Swagger/OpenAPI (213 operations), Caddy,
   and post-rehost startup checks pass. The prior binary is retained at
   `builds/evcmsnew.pre-deployed-d635446-20260828-164259` for rollback.

### 2026-08-28 - Live charging financial and usage projections deployed

- Customer session detail, CPO customer reads, and CPO live-session snapshots
  now expose current accrued usage/amount projections from immutable tariff and
  tax snapshots. Projected amounts remain separate from final settlement
  totals, and all reads remain tenant/customer scoped. No migration or data
  repair was required.
- Runtime revision `d63544641e257436d988a415ae069a7d8baeeb2f` is active behind
  Caddy with binary SHA-256
  `bab0d777d3d0e2f467ba4cdbf939a8913b43bf18ae4f4cf813fb550d640cc338`.
  Service/readiness, public routing, Swagger/OpenAPI (213 operations), Caddy,
  and post-rehost startup checks pass. The prior binary is retained for
  rollback.

### 2026-08-28 - CPO customer aggregate wallet reads deployed

- CPO customer aggregates now load session usage/counts and wallet balance in
  separate queries, so missing wallet rows and zero-session customers return
  stable zero values without masking either aggregate. No migration or data
  repair was required.
- Runtime revision `b6b723d` is active behind Caddy with binary SHA-256
  `92cadd70b9b18106ddf416b3ee06a752ac2c0fc706576fe4e37c6099186decbd`.
  Service/readiness, public routing, Swagger/OpenAPI (213 operations), Caddy,
  and post-rehost startup checks pass. The prior binary is retained for
  rollback.

### 2026-08-28 - Support core and notifications deployed

- Migration 057 converts historical ticket `PENDING` state to `IN_PROGRESS`,
  adds immutable ticket events, and preserves ticket/message state as the
  durable authority. Platform status changes are graph-validated and locked;
  CPO replies reopen resolved/closed tickets with a separate truthful status
  event and clear `closed_at`.
- Support queues are bounded summary pages with timestamp-plus-ID cursors,
  tenant/status/search filters, last-message metadata, and no thread bodies.
  Detail returns the ticket, ordered messages, and lifecycle history. Replies
  have durable per-ticket idempotency keys and lock the current row before any
  mutation. Request bodies are bounded, one-object, unknown-field-rejecting
  JSON.
- Source OpenAPI and the SuperAdmin support workflow guide describe the new
  contract. Ticket creation, platform replies, and platform
  resolved/closed/reopened transitions queue encrypted CPO notices atomically;
  CPO-created/replied/reopened activity emits durable platform events rather
  than guessing a platform support mailbox. Notifications expose only subject,
  status, localized time, and the configured action URL—not private message
  bodies. Support core and notification completion are published at
  `256aa8975fa07dc032dd779c8eb4b0d93a3b1a73` and migration 56/57 are applied.
  Runtime revision `b6b723d` is active behind Caddy with binary SHA-256
  `92cadd70b9b18106ddf416b3ee06a752ac2c0fc706576fe4e37c6099186decbd`.
  Service/readiness, public routing, Swagger/OpenAPI (213 operations), Caddy,
  and post-rehost startup checks pass. The migration ordering correction for
  existing `PENDING` tickets was applied before migration 57. SMTP delivery,
  physical charger acceptance, and disposable PostgreSQL lifecycle coverage
  are not claimed; `pwsh` and `TEST_DATABASE_URL` are unavailable on this VPS.

### 2026-08-26 - Independent charging-limit provenance deployed

- Customer execution intent is now independent from tariff billing dimension.
  CMS can send simultaneous energy and duration thresholds when a customer
  bound and the tariff-dimension wallet guard differ; every AUTO/ENERGY/TIME/
  MONEY selection is valid for energy, time, and session tariffs.
- Migration 055 adds durable source fields for each effective threshold and is
  applied in `devevcmsnewdb`. CMS final settlement remains based on actual
  evidence and frozen tariff/tax snapshots; the wallet buffer remains a
  one-time finite metered-stop margin, not a bill cap.
- Runtime revision `e8ff810` is active behind Caddy with binary SHA-256
  `c1576ffb163b474ea3e3e984174ba6b02648607a7951917513bbb159e78021ba`.
  Service readiness, public routing, Swagger/OpenAPI, Caddy validation, and
  post-rehost startup checks pass. The pre-migration database dump is retained
  at `/root/evcmsnew-backups/devevcmsnew-before-000055-20260826-150844.dump`.
  No physical charger acceptance is claimed; disposable PostgreSQL and `pwsh`
  documentation verification remain unavailable on this VPS.

### 2026-08-26 - Customer-selected and wallet-derived charging limits

- Customer starts now use one durable admission path for AUTO, ENERGY, TIME,
  and MONEY execution limits. Omitted limits derive a metered threshold from
  usable wallet balance without a one-hour default; tariff billing semantics
  remain independent.
- The persisted intent carries the requested classification/value and effective
  energy/deadline bounds. Metered holds include the configured wallet buffer,
  intentionally covering bounded meter/RemoteStop overshoot.
- CMS sends this metadata through the established authenticated HAL start
  command. Migration 54 is applied in `devevcmsnewdb`; runtime revision
  `a085f29` is active behind Caddy with binary SHA-256
  `ff6341384fc85c11191c709edc8fdba6f3f27207d0a2af4bb36069636544e9e5`.
  Service readiness, public routing, Swagger/OpenAPI, Caddy validation, and
  post-rehost startup checks pass. The pre-migration database dump is retained
  at `/root/evcmsnew-backups/devevcmsnew-before-000054-20260826-111224.dump`.
  No physical charger acceptance is claimed; disposable PostgreSQL and
  `pwsh` documentation verification remain unavailable on this VPS.

### 2026-08-26 - SuperAdmin support-desk documentation reconciled

- The existing durable support-ticket API now has a complete SuperAdmin
  workflow guide covering platform/CPO authority, persistence/order, actual
  reply/status semantics, recovery, privacy, and explicit product limits. It
  is linked from the documentation index, SuperAdmin handoff, control-plane
  concept, shared HTTP contract, and permission matrix.
- The source behavior is unchanged. OpenAPI now accurately requires both the
  CPO bearer session and `X-CPO-App-ID` for CPO support operations, matching
  runtime middleware; platform support remains `PLATFORM` bearer-only. No
  migration, data mutation, deployment, or live-environment verification is
  claimed by this documentation slice.
- Local documentation verification, focused OpenAPI/runtime route parity,
  serial `go test -p 1 ./...`, and serial `go vet -p 1 ./...` passed. The
  documentation-only slice does not establish a deployed support-desk state.

### 2026-08-25 — CPO live-session display-context rehost verified

- Source live-session rows now include `duration_seconds` at the response
  `as_of`, CPO-visible `customer_name`, and canonical CMS `connector_id`.
  Customer identifiers/contact details and commercial fields remain excluded.
- Runtime revision `e6c0ebb` is active behind Caddy with binary SHA-256
  `52cdb19f91e5ea3403417dba82ebcc2a795793ff71ee73ae1bcfd8ea20d41acf`.
  No migration or database mutation was required. Service readiness, public
  routing, protected live-session routes, Swagger/OpenAPI, Caddy, and
  post-rehost startup checks passed; the prior binary is retained for rollback.

### 2026-08-25 — CPO analytics period filters rehost verified

- CPO analytics accepts optional `period`/`date` filters for session revenue,
  usage, and count while charger/connector totals remain overall. The query is
  tenant-scoped and uses existing persisted session data; no migration or data
  repair was required.
- Runtime revision `f432f45` is active behind Caddy with binary SHA-256
  `3de4d28e712e31cb2e06c56a7b39570bcecaa5580dcd6710e7f910f8cd9ae4fe`.
  Service readiness, public routing, protected analytics access, Swagger/raw
  OpenAPI, Caddy, and post-rehost startup checks passed.

### 2026-08-25 — CPO full-snapshot live-session SSE rehost verified

- Source now makes `GET /api/v1/cpo/operations/live-sessions` the primary CPO
  ADMIN/app-ID-scoped SSE. It immediately sends the complete current
  `LiveChargingSessionListResponse` as `snapshot` and sends a complete
  replacement `live_sessions` payload after committed session/meter/SoC changes.
  The UI therefore does not consume raw operational events or assemble state
  from invalidations.
- `GET /api/v1/cpo/operations/live-sessions/snapshot` retains the JSON
  recovery/keyset-pagination contract. The filtered retained event route is
  advanced reconciliation tooling, and the old realtime path is a deprecated
  compatibility alias. Runtime revision `d3ac043` is active behind Caddy with
  binary SHA-256
  `0595788804141ccea4972be4abd33b06fa1031ae50a2720df3e357725c6738b6`.
  The service is enabled and healthy; migrations remain through 53, the source
  contract serves 210 operations, and public/loopback readiness, Swagger,
  protected stream/snapshot routes, Caddy, and post-rehost startup checks pass.
  No migration or database mutation was required.

### 2026-08-25 — CPO customer total-usage projection repair rehost verified

- Source now maps the CPO customer list/detail energy aggregate alias
  `total_usage_kwh` explicitly. Completed/reconciliation session energy already
  persisted in `charging_sessions.total_kwh` will therefore populate
  `total_usage_kwh` consistently with `session_count`; no schema or data change
  is required.
- Runtime revision `849d80b` is active behind Caddy with binary SHA-256
  `0c8ca55b819073e905fe09a16345972ec819a2836531c9ab10366d6e433aaf2b`.
  No migration or data repair was required. Service readiness, public routing,
  Swagger/OpenAPI, Caddy, binary identity, and post-rehost checks passed; the
  prior binary is retained for rollback.

### 2026-08-25 — CPO charging-session charger projection repair rehost verified

- Source now protects CPO charging-session history and live-session reads from
  incomplete nested GORM charger preloads. A missing direct session relation is
  resolved from the connector's CPO-owned charger in one bounded lookup,
  preserving human-readable charger/hub projection without a database write.
  If neither persisted relation resolves, the response remains unresolved rather
  than fabricating a charger.
- Runtime revision `0fd9774` is active behind Caddy with binary SHA-256
  `214d0c60bab30610ac4174ea8f46644b4b5cd26f0c74dc0fe86c9208b6191aed`.
  No migration or database mutation was required. The tenant-scoped fallback,
  service readiness, public routing, Swagger/OpenAPI, Caddy, and post-rehost
  startup checks passed; the prior binary is retained for rollback.

### 2026-08-25 — CPO live-session operations rehost verified

- Source now exposes a CPO ADMIN/app-ID-scoped ongoing-session snapshot at
  `/api/v1/cpo/operations/live-sessions`, plus filtered durable replay and SSE
  companions. The snapshot contains only materialized `ACTIVE`,
  `STOP_PENDING`, and `RECONCILIATION_REQUIRED` sessions with human-readable
  charger/hub context and committed meter/SoC freshness; it never calls HAL or
  exposes customer, wallet, tariff, or settlement data. REST remains the
  recovery authority after a retained `CHARGING_SESSION` invalidation.
- Customer charging/recharge admission now gives the one PostgreSQL-enforced
  current subscription precedence over terminal subscription history. A prior
  expired row therefore cannot keep a newly renewed active CPO blocked; an
  expired current subscription still blocks only new customer paid commands.
- Runtime revision `aa44e4b` is active behind Caddy with binary SHA-256
  `b8bbf2453a1f84ffef0d38855b0406a73be1ecca6ee590f6a4727811af8ebe32`.
  The service is enabled and running, migrations remain through 53, the live
  session route returns the expected unauthenticated 401, and loopback/public
  live/readiness, Swagger, raw OpenAPI, and Caddy checks pass. The source
  OpenAPI serves 209 operations. No database mutation was required.

### 2026-08-25 — CPO legal-identity migration and rehost verified

- Migration 53 is applied in the development deployment together with
  migrations 49–52. Its GSTIN, GSTIN-state, and PIN checks remain `NOT VALID`
  so the three existing legacy CPO rows are unchanged; new and updated rows
  are protected. The live database confirms all three constraints are present
  and unvalidated, and the legacy rows remain present.
- Runtime revision `162b3be` is active on `127.0.0.1:18080` behind Caddy with
  binary SHA-256
  `0dcdc9adc5c93d68fdeaf1df36e6ec18d9345b8a50200ed71f2d0fa433a47c83`.
  `/health/live`, `/health/ready`, `/docs/`, and `/openapi.yaml` return 200
  locally and through `https://dev-evcmsnew.transev.site`; the service is
  enabled, Caddy validates, and no recent service errors were found.
- A rollback database dump and prior binary are retained for this deployment.

### 2026-08-25 — CPO and SuperAdmin frontend contracts reconciled in source

- `CPO_FRONTEND_INTEGRATION_HANDOFF.md` now provides the browser-facing CPO
  causal model: trusted scope/app-ID bootstrap, complete routed API inventory,
  commercial/network invariants, permission-editor limits, operational
  replay/SSE recovery, and tenant-safe error handling.
- `SUPERADMIN_CPO_FRONTEND_BOUNDARY.md` separates the two frontend planes, and
  `contracts/api/superadmin-permission-matrix.md` manually classifies all 65
  current `/api/v1/platform/*` operations plus the 12 shared administrative
  auth operations. Every platform operation remains enforced by the single
  server-side `PLATFORM` authority; classification is not new granular RBAC.
- Current source uses capability-protected CPO routes and tenant-scoped
  support/notification endpoints. Every CPO request retains verified app-ID
  and active-membership enforcement; no deployment or migration was performed
  by this source slice.

### 2026-08-25 — CPO staff authority and human-readable charging projections in source

- CPO create/profile writes now normalize human text, require meaningful CPO
  and administrator names, verify Indian GSTIN structure/checksum, require its
  state code to match the CPO registration state, and require a six-digit Indian
  PIN. Migration 53 preserves existing legacy rows and adds equivalent
  PostgreSQL constraints as `NOT VALID`; new and updated rows are enforced.
  Normalized GSTIN remains globally unique, so no redundant GSTIN/business-name
  compound key exists; legal-name ownership remains unverified without a GST
  registry integration.
- Additive migrations 49 and 50 define per-membership CPO permission overrides
  and snapshot multi-CPO announcement targeting; both are applied in the
  development deployment.
- CPO staff can be listed, created, updated, activated, suspended, and revoked
  through ADMIN-protected routes. Non-ADMIN memberships can authenticate when
  active; the catalog and override evaluator are source-controlled, with
  explicit DENY precedence.
- CPO customer lists always serialize `total_usage_kwh`. Charging-session and
  charger-transaction projections now include stable CMS, HAL/OCPP, human
  charger/hub, connector, payment, settlement, and reconciliation fields.
- Platform announcements accept multiple CPO targets and snapshot both targets
  and recipients at publish time. The existing encrypted mail outbox now
  rejects unknown templates and SMTP emits text plus safe HTML alternatives.
- The `subscription-lifecycle` worker is current-worker observed like the
  other CMS workers. It records 7/3/1-day thresholds exactly once and expires
  elapsed manual subscription periods without changing CPO lifecycle status.
  Expiry (or an elapsed period before worker projection) blocks only new
  customer starts and new wallet recharge orders. SuperAdmins renew the same
  expired record through the existing audited/idempotent renewal command;
  customer stop, reconciliation, settlement, read, and pre-expiry recharge
  verification remain available.
- Source-controlled mail layouts are embedded from `src/mail/templates/` and
  produce text plus HTML alternatives. CPO support tickets/messages are durable
  tenant-scoped records; CPO routes cannot read another CPO's tickets while
  platform routes provide the status/response control plane. Ticket creation,
  platform replies, and platform resolved/closed/reopened transitions enqueue
  privacy-safe CPO notification intents atomically with their support mutation;
  CPO-originated activity is retained in the platform event stream without a
  hard-coded platform support mailbox.
- Focused auth/CPO/SuperAdmin/mail tests, docs validation, OpenAPI route parity,
  `go test ./...`, and `go vet ./...` pass. Disposable PostgreSQL migration and
  lifecycle verification remain unrun because `TEST_DATABASE_URL` was not set.

### 2026-08-25 — Optional OCPP SoC telemetry source change

- CMS accepts the additive immutable HAL `transaction.soc` fact and stores
  nullable first/latest charger-observed percentages, actual observation time,
  and independent sequence on `charging_sessions`. It never derives SoC from
  energy, capacity, completion, or a default value.
- Customer live detail has `soc_percent`, `soc_observed_at`, and independent
  `soc_freshness`; customer and CPO history expose first/last observed values.
  A SoC-only accepted fact emits `charging.telemetry_changed` for REST refresh.
- Additive migrations 48–53 are applied in the development deployment. Runtime
  revision `162b3be` is active with binary SHA-256
  `0dcdc9adc5c93d68fdeaf1df36e6ec18d9345b8a50200ed71f2d0fa433a47c83`; the
  pre-migration database dump and prior binary are retained under
  `/root/evcmsnew-backups/` and `builds/`.
  Focused source tests, full Go tests, vet, and route/OpenAPI verification pass.
  Physical cpconsole acceptance and disposable PostgreSQL lifecycle tests
  remain unverified without a disposable topology.
- The deployed CPO session reads now include stable customer, charger, hub, and
  connector identity/details and overlay live consumed kWh for active sessions.
  Incomplete session relations fall back through connector-owned charger
  hydration. SuperAdmins can use the customer-intelligence route for CPO-level
  customer metrics and top-customer summaries; these changes require no new
  migration.

### 2026-08-21 — Real-hardware CMS/HAL hardening in source

- CMS mutation correlation is now canonical UUID-only at the HAL client boundary.
  Mapping reconciliation creates a fresh UUID per attempt, and migration 47
  adds bounded safe mapping-failure diagnostics; it has not been applied to a
  database in this slice.
- CMS forwards optional charger serial evidence without changing OCPP identity.
  Start admission now permits only fresh online raw `Available` or `Preparing`;
  the established `Preparing -> CHARGING` display projection is unchanged.

Focused and broad Go tests, vet, and documentation verification pass. Database
migration/lifecycle and physical-device acceptance remain unrun without an
explicitly disposable database and hardware session; no database, deployment,
Caddy, or live service was changed.

### 2026-08-21 — CMS/HAL signup and fact-recovery closure deployed

- A new customer signup Start now atomically invalidates and scrubs the prior
  current CPO/normalized-email challenge before inserting its replacement and
  durable mail outbox record. Migration 46 remains the database backstop.
- Platform superadmins can request HAL requeue of an exact terminal fact only
  through `POST /api/v1/platform/hal-facts/{fact_id}/requeue`. CMS uses the
  server-generated request UUID for correlation, records audit evidence, and
  emits an event only after HAL accepts the immutable fact back to `PENDING`.
  Runtime revision `c6b79d4` is active; no migration or database mutation was
  required.

Verification: focused CMS adapter/platform/route/OpenAPI/customer-auth tests,
full Go tests, vet, and diff checks passed. PostgreSQL signup races remain
skipped without `TEST_DATABASE_URL`. Binary SHA-256 is
`9a1151f94c34ef9518a3067d9192a28e7c4d9843a2b8564b28399bb9195c8b78`.
Local/public health-readiness, Swagger, raw OpenAPI (189 operations), Caddy
validation, and the post-rehost journal scan passed. `pwsh` remains unavailable
on this Ubuntu host.

### 2026-08-20 — Auth/current-worker invariant hardening deployed

- Migration `000046_auth_challenge_and_readiness_invariants` guards against
  pre-existing duplicate current challenges, then adds partial unique indexes
  for administrative, customer, and signup identities. It makes no attempt to
  select a winner or erase challenge history. It also rejects unknown GST
  states before requiring the domain-owned `gsts.state` column to be non-null.
- Challenge issue, resend, verification, and password-reset flows serialize on
  their user/customer identity; signup uses an advisory transaction lock for
  its CPO/normalized-email identity. Login repeats credential/state validation
  after that lock, and concurrent current-password changes are serialized.
- `/health/ready` now requires the exact enabled worker instances expected by
  this process. Platform maintenance always blocks readiness; HAL reconciliation
  does so only when HAL is enabled, mail only when mail is enabled, and
  operational-event retention remains observable but non-blocking.
- The wallet-recharge payment-method model now preserves a provider omission as
  `NULL`; `connectors.connector_max_capacity` remains intentionally retained
  historical schema residue. Charging migration 45 and its `total_kwh` repair
  are unchanged.

Verification: focused auth/customer-auth/platform-ops checks, the complete
`go test -p 1 ./...` suite, `go vet -p 1 ./...`, and `git diff --check` passed.
The new PostgreSQL migration/concurrency tests are present but skipped because
no disposable `TEST_DATABASE_URL` was supplied. A verified pre-migration backup
was created and migration 46 was applied. Runtime revision `166b0de` is active
with binary SHA-256
`07342a2b1a0aa884b12c5f08f24d6721d5946a3db05080b773ed1f882c013b96`.
Local/public health-readiness, Swagger, raw OpenAPI (188 operations), Caddy
validation, and the post-rehost journal scan passed. `pwsh` remains unavailable
on this Ubuntu host.

### 2026-08-20 — Charging lifecycle reconciliation deployed

- Migration `000045_charging_session_occupancy_and_reconciliation` transfers
  connector ownership from an unmaterialized start intent to one occupying
  session (`ACTIVE`, `STOP_PENDING`, or `RECONCILIATION_REQUIRED`). It rejects
  pre-existing conflicting data rather than rewriting it. Historical
  `ACTUALLY_STARTED` remains valid start evidence and no longer blocks a later
  session on its own.
- Completion facts durably record terminal meter/time/amount evidence before
  settlement. An unsafe settlement keeps the session and wallet hold in
  `RECONCILIATION_REQUIRED`; bounded transactional retry uses the session's
  stable payment/ledger identity and cannot debit twice. Start admission now
  subtracts held and reconciliation reservations while the wallet row is locked.
- Exact HAL STOP reconciliation is now session-aware: provider rejection or
  confirmed absence returns an incomplete session to `ACTIVE`; ambiguity stays
  `STOP_PENDING`, and completion wins. HAL transaction lookup rejects malformed
  successful payloads, including zero identities.

Verification: focused charging/HAL/CPO/route checks, full `go test -p 1 ./...`,
`go vet -p 1 ./...`, and `git diff --check` passed. The disposable PostgreSQL
occupancy tests are skipped because `TEST_DATABASE_URL` is unset. Migration 45
was backed up and applied successfully; runtime revision `765cc80` is active
with binary SHA-256
`33a99ca227261065365b00473875f48ade845eb1d72f3a2b21371e2e6dfc412b`.
Local/public health-readiness, Swagger, raw OpenAPI (188 operations), Caddy
validation, and the post-rehost journal scan passed. `pwsh` remains
unavailable on this Ubuntu host.

### 2026-08-20 — ChargingSession `total_kwh` model correction deployed

- `models.ChargingSession.TotalKWh` explicitly maps to the canonical,
  migration-owned `charging_sessions.total_kwh` column. It no longer relies on
  GORM's acronym splitting, which inferred the nonexistent `total_k_wh` and
  caused PostgreSQL `42703` during persistence.
- The regression suite parses the migration-aligned acronym-sensitive fields
  and builds a PostgreSQL-dialect dry-run insert that contains `total_kwh` and
  excludes `total_k_wh`. No migration or database mutation was required.
  Runtime revision `765cc80` is active with binary SHA-256
  `33a99ca227261065365b00473875f48ade845eb1d72f3a2b21371e2e6dfc412b`.
  Local/public health-readiness, Swagger, raw OpenAPI (188 operations), Caddy
  validation, and the post-rehost journal scan passed.

### 2026-08-20 — HAL command-response contract hardening deployed

- CMS now rejects 2xx HAL command responses without a canonical `command`
  wrapper, nonzero/matching HAL and CMS IDs, supported kind/state, valid
  timestamp, and nonzero optional transaction identity. It cannot persist
  `uuid.Nil` from a malformed provider response.
- Synchronous START and STOP command identity persistence and exact-command
  reconciliation use that fail-closed adapter. Authoritative start evidence
  now treats a stored zero UUID as unknown, repairs it to HAL's real nonzero
  identity, and materializes normally; only two different nonzero IDs conflict.

Verification: documentation verification, focused HAL-client/charging checks,
full `go test ./...`, `go vet ./...`, and `git diff --check` pass. The new
disposable PostgreSQL recovery regression is skipped while `TEST_DATABASE_URL`
is unset. Runtime revision `1729b41` is active with binary SHA-256
`00bcedfc71f42e0ce45cd8df99e1616598d03265ce291fd5e08d9cf8338ddb32`.
Migrations remain through `000044_temporal_tariff_fallback.up.sql`; no
migration or data mutation was required. Local/public health-readiness,
Swagger, raw OpenAPI (188 operations), Caddy validation, and the post-rehost
journal scan passed. `pwsh` remains unavailable on this Ubuntu host.

### 2026-08-19 — HAL authoritative-start recovery and fact classification source change

- Expected immutable HAL fact rejections now return stable 4xx/409 errors,
  while only unexpected projection/persistence failures return retryable 500.
  Safe debug diagnostics classify error type, failure class, and SQLSTATE where
  available without logging fact contents or credentials.
- CMS now uses HAL's exact transaction-by-start-intent lookup for old accepted,
  protocol-acknowledged, and reconciliation-required starts. Returned truth is
  processed through the same locked materializer as `transaction.started`; a
  404 cannot create a session. Late truth after a terminal CMS intent is
  retained and made reconciliation-visible rather than silently discarded.
- The synchronous start-command response no longer regresses a concurrent
  fact-materialized `ACTUALLY_STARTED` intent. No migration was required.

Verification: focused database-free HAL-client/config/fact packages pass.
Disposable PostgreSQL state-machine and dual-service virtual-charger acceptance
remain unverified because `TEST_DATABASE_URL` and the topology are unavailable.

### 2026-08-19 — CMS-generated HAL mutation correlation deployed

- User App charging Start and Stop take the canonical CMS `RequestLogger` ID
  from Gin context for all HAL mapping/start/stop mutations. They do not depend
  on an incoming `X-Request-ID`; a client-supplied value is not the internal
  CMS-to-HAL correlation authority.
- `halclient` rejects an empty or whitespace-only mutation correlation before
  sending HTTP. Command lookup remains correlation-free, and charging
  reconciliation, OCPP truth, wallet, tariff, and GST behavior are unchanged.
- No migration was required. Runtime revision `d7f72cd` is active with binary
  SHA-256
  `66a8416d81fc54d397d42c8d4cacb1a866dfa1cc934b5cc6c1ba2b00a46b5754`.
- Focused correlation/client/route tests, full Go tests, and vet passed. Local
  and public health/readiness, Swagger, raw OpenAPI (187 operations), Caddy
  validation, and post-rehost logs passed. `pwsh` and disposable
  `TEST_DATABASE_URL` remain unavailable.

### 2026-08-19 — Charging-start prerequisite and exact-absence recovery deployed

- A new customer start first synchronizes the exact HAL charger/connector
  mapping. A preflight failure returns `503 charger_mapping_unavailable`
  without creating a commercial intent, wallet hold, or HAL command record.
  A second post-transaction confirmation closes the inventory-change race; its
  known pre-delivery failure terminalizes/release-cleans the just-created
  attempt rather than reporting delivery reconciliation.
- `RECONCILIATION_REQUIRED` now means only that `RequestStart` was invoked and
  its result is unresolved. Exact HAL GET by the original `cms_command_id`
  projects a found command; its canonical 404 atomically moves an unmaterialized
  start to `REJECTED` (or `EXPIRED` after command expiry), marks the command
  `CONFIRMED_ABSENT`, and changes only `HELD` to `RELEASED`. Other lookup
  failures retain reconciliation. STOP remains distinct: missing STOP evidence
  never completes a session or settles money.
- The appv1 credential remains transient and SHA-256-hashed only in CMS. No
  schema migration was required. Runtime revision `6f65e8e` is active with
  binary SHA-256
  `e675c0acd9e77ba7e3293f422951f8f4b056881b198327119148738f2536711c`.
- Focused charging/reconciliation and route/OpenAPI tests, full Go tests, and
  vet passed. Local/public health/readiness, Swagger, raw OpenAPI (187
  operations), Caddy validation, and post-rehost logs passed. The disposable
  PostgreSQL reconciliation test remains skipped because `TEST_DATABASE_URL`
  is unset; `pwsh` is unavailable.
### 2026-08-18 — CPO wallet transaction read deployed

- Added authenticated, tenant-scoped `GET /api/v1/cpo/wallet-transactions` for
  CPO ADMINs. It returns newest-first wallet transaction projections for the
  authenticated CPO, with optional customer filtering and bounded keyset
  pagination (`limit`, `before`, `before_id`).
- No database migration was required. Runtime revision `2a040e0` is active with
  binary SHA-256
  `1a5dfc0100f85a6159c916880911c5139b5295a7b0a074b42e92668f29a0dc3e`.
  The live OpenAPI contract contains 187 operations.
- Local/public health and readiness, Swagger, raw OpenAPI, the new route's
  unauthenticated boundary (`401`), Caddy validation, and post-rehost logs
  passed. `TEST_DATABASE_URL` and `pwsh` remain unavailable.

### 2026-08-18 — Temporal tariff fallback and hub cleanup deployed

- Source migration 44 replaces the prior active-tariff no-overlap exclusion
  with deterministic root/open-ended/bounded fallback per immutable exact
  target. Scope precedence remains `USERGROUP > CHARGER > HUB`; time resolves
  only inside the first matching scope. These dates are commercial applicability
  and do not alter customer session-duration cutoff behavior.
- CPO scoped tariff create/PATCH/delete and Hub publication enforce the enabled
  topology and the requirement that a customer-visible Hub has exactly one
  enabled unbounded Hub root. User App informational price and new start
  admission share the resolver and preserve immutable tariff/tax snapshots.
- The source release-blocker follow-up rejects visible Hub creation before
  insert, maps the Hub root-floor database guard to the same `409`, and makes
  tariff target identity immutable in migration 44. Resolver topology failures
  fail closed as commercial unavailability, while query/infrastructure failures
  propagate to normal error handling rather than being mislabeled as no tariff.
- A final concurrency correction makes the Hub publication trigger
  acquire the same transaction advisory key as Hub tariff topology mutation:
  `tariff:<cpo_id>:hub:<hub_id>`. Direct concurrent publication and root
  deactivation/deletion can no longer both commit into a visible Hub without a
  root tariff.
- Migration 44 is applied to the development database after a mode-0600
  rollback dump at `/var/backups/postgres/devevcmsnewdb-pre-hub-cleanup-migration-44-20260818-162307.dump`
  (SHA-256 `c64597066e052c83e7f60edc14497727acd9ad89ba628343a462696ae8f9a66e`).
- The two requested hubs (`63ddfa1f-0c1e-4131-8ad7-eb5835c7cd19` and
  `e00412ec-785e-4c71-85f8-9cb4e30a2d29`) and their hub tariffs/link rows were
  removed. Their five chargers and connectors were retained as inventory but
  unassigned, hidden, and set `INACTIVE`; no charging sessions existed for
  them.
- Runtime revision `38625d9` is active with binary SHA-256
  `70626eb4cca88cfcb3a90590b656e197c54bf8c152d198942f756b4146f9101e`.
  The live OpenAPI contract contains 186 operations. Local and public health,
  readiness, Swagger, raw OpenAPI, Caddy validation, and post-rehost log checks
  passed. Disposable PostgreSQL lifecycle tests still require an explicitly
  selected `TEST_DATABASE_URL`; `pwsh` is unavailable.

### 2026-08-18 — CPO charger transactions deployed

- Added authenticated, tenant-scoped `GET /api/v1/cpo/charger-transactions`
  with descending cursor pagination and optional same-CPO charger/customer
  filters. The read returns billing, usage, tariff, hub, host, customer, and
  financial-status projections without contacting the HAL.
- The authoritative OpenAPI contract now includes the full financial-status
  enum, including `REFUNDED`, and the transaction response schemas.
- No database migration was required. Runtime revision `a5d1af4` is active
  with binary SHA-256
  `c786df05b310487fb84c6a6f85ab4c769249396f32ac6d0f027efb8c0b273669` and
  the live contract expanded to 183 operations.

### 2026-08-18 — Tariff PATCH and frozen-settlement hardening deployed

- Tariff PATCH now distinguishes omitted, explicit-null, and concrete values
  for `units`, `start_date`, and `end_date` across Hub, Charger, and UserGroup
  scopes. `units:null` is the supported sessions transition; both date fields
  must be null together to clear an existing schedule. Each update validates
  the resulting locked row before persistence.
- Completion settlement validates the immutable SGST/CGST/IGST snapshot using
  shared commercial component semantics and never reads current Hub/GST state
  to price an existing session. Legacy frozen tariff snapshot compatibility is
  unchanged.
- No migration or runtime database mutation was required. Runtime revision
  `0ad2de7` is active with binary SHA-256
  `d62b01cad7b25bd4ddd0c82407ce7ee94dd7d03680879edb57057cbd526b9348`.
- Focused tariff/GST/route tests, full Go tests, vet, Caddy validation,
  local/public health/readiness, Swagger, raw OpenAPI, auth boundaries, and
  post-rehost log checks passed. Disposable PostgreSQL lifecycle tests still
  require an explicitly selected `TEST_DATABASE_URL`; `pwsh` is unavailable.

### 2026-08-18 — CPO analytics and hub charger listing deployed

- Added authenticated, tenant-scoped `GET /api/v1/cpo/analytics` with charger,
  connector, session, revenue, and energy-usage aggregates.
- Added authenticated, tenant-scoped `GET /api/v1/cpo/hubs/{hub_id}/chargers`
  for listing chargers attached to a CPO hub. Both reads are side-effect free
  and do not contact the HAL; cross-tenant hub IDs remain hidden as
  `404 hub_not_found`.
- Corrected the authoritative analytics response schema to match the runtime:
  counts are non-negative integers and decimal revenue/usage values serialize
  as strings.
- No database migration was required. Runtime revision `a5d1af4` is active with
  binary SHA-256
  `d62b01cad7b25bd4ddd0c82407ce7ee94dd7d03680879edb57057cbd526b9348` and
  the live OpenAPI contract contains 182 operations.
- Focused route/OpenAPI tests, full Go tests, vet, Caddy validation, local/public
  health/readiness, Swagger, raw OpenAPI, auth boundaries, and post-rehost log
  checks passed. The PowerShell documentation verifier remains unavailable
  because `pwsh` is not installed.

### 2026-08-18 — CPO wallet admission policy deployed

- Source migration 43 gives every existing CPO a blank `settings` row without
  overwriting existing invoice or wallet-policy values; new CPO provisioning
  creates the same row transactionally. Both wallet policy fields default to
  zero and are exposed through the tenant-scoped CPO settings API.
- Every new customer charging start locks the CPO, wallet, and settings row,
  rejects `balance < wallet_min_balance`, then calculates the tariff/GST hold
  and HAL Wh limit from `balance - wallet_buffer_min_balance`. The buffer is
  not a second admission threshold. Existing start intents, sessions, and
  settlement snapshots are unchanged.
- Customer wallet and wallet-history reads expose the current CPO minimum and
  buffer alongside exact `usable_balance` and a threshold-only
  `minimum_recharge_amount`; these are current read projections, while start
  admission remains authoritative at its locked transaction boundary.
- Migration 43 was applied to the development database after a mode-0600
  rollback dump at `/var/backups/postgres/devevcmsnewdb-pre-migration-42-43-20260818-123600.dump`.
  The development service now has settings rows for all existing CPOs.
- Runtime revision `ceefb21` is active with binary SHA-256
  `b326c525d0a11a1a1d152f60a08c1906bfa50dc00af0e4db7f080f17b59636e1`.
- The guarded PostgreSQL admission test still requires an explicitly selected
  disposable `TEST_DATABASE_URL` and was not run against the live database.

### 2026-08-18 — Tariff/GST commercial correction deployed

- Migration `000040_rename_tariff_price_per_unit` preserves tariff values while
  naming the canonical durable field `price_per_unit`; current writes reject
  `price_per_kwh`. The source forward migration 42 renames the durable
  enum value `watt/hour` to `kwh` without changing any numeric price: an energy
  value such as 16.91 means 16.91 per kWh, and meter Wh are divided by 1000.
- Supported fixed charging tariffs are explicit: energy per `kwh`, time per
  `minutes`, or one fixed `sessions` amount. Customer price, admission holds,
  frozen start-intent/session snapshots, and settlement use that one shared
  interpretation. GST remains Hub-owned and independent from tariff targeting;
  precedence is `USERGROUP > CHARGER > HUB`.
- Time pricing uses the actual session duration from the HAL start/complete
  facts. It does not alter the independent existing customer/HAL duration
  cutoff. New snapshots contain their semantic fields; historical snapshots
  with `price_per_kwh` are read only through a deliberately named legacy path.
- New active tariffs reject a non-zero idle fee because no authoritative idle
  interval exists; zero remains a durable audit/snapshot value and is never
  billed. Existing non-zero active records are unavailable for pricing rather
  than silently under-billed.
- Migration 42 was applied to the development database together with the
  guarded duplicate-assignment check. The persisted energy enum is now `kwh`,
  and the partial unique CPO/GST hub-assignment index is present.
- Disposable PostgreSQL lifecycle verification remains pending an explicitly
  selected `TEST_DATABASE_URL`.

### 2026-08-18 — Wallet aggregates and settings migration deployed

- Migration 41 adds non-null `settings.wallet_min_balance` and
  `settings.wallet_buffer_min_balance` columns with zero defaults.
- CPO customer aggregate loading now starts from all customers in the tenant,
  so wallet balance and zero usage/session values are present even when a
  customer has no charging-session rows.
- Revision `040b9bb` is active with binary SHA-256
  `2b4a0ab8fd77b79ec92bf03a418acaa52eb6320e0e90401c62afbb9d23fced29`.

### 2026-08-18 — Tariff unit-price semantic correction deployed

- Migration 40 is applied on the development VPS after a mode-0600 rollback
  dump. Revision `9e7af67` is active with the then-current tariff contract.
- The source correction above superseded the former `watt/hour` energy
  interpretation and is now included in the migration-controlled deployment.
  Disposable PostgreSQL lifecycle and full CMS-to-HAL topology acceptance
  remain pending their dedicated environments.

### 2026-08-18 — Customer aggregates and tariff validation release deployed

- CPO customer list/detail responses now include tenant-scoped total usage,
  session count, and wallet balance aggregates.
- Tariff type, price type, and unit fields are validated before persistence;
  unsupported values return request validation errors instead of database enum
  failures. The OpenAPI contract no longer advertises unregistered tariff/GST
  DELETE operations.
- No database migration was required; migration 39 remains the latest applied
  migration. Revision `d475b41` is active with binary SHA-256
  `1ff0938cdbc3fda7b181ac7f788daae8620ecdffc00f060cc227b5e73bae24ba`.
- The enabled service is active with zero restarts. Caddy validation, focused
  and full Go tests, vet, local/public health/readiness, Swagger, raw OpenAPI,
  and unauthenticated worker/status boundaries passed. The live contract
  remains at 180 operations.

### 2026-08-14 — Worker current-instance projection deployed

- Migration `000039_make_worker_current_instance_explicit` retains durable
  `worker_instances` history while selecting one `is_current` process
  incarnation per logical worker name. A replacement registration atomically
  supersedes the prior current row; delayed heartbeats from old rows are no-ops.
- Platform workers, SuperAdmin overview/status, and `GET /api/v1/platform/workers`
  now report only the current projection. Current heartbeats derive `STALE` at
  read time; readiness evaluates that current required instance rather than raw
  historical rows.
- Migration 39 is applied on the development VPS after a mode-0600 rollback
  dump. Revision `11c4c23` is active with binary SHA-256
  `2aa8f6c5cd8e0053a72e36d400c2da87ec84e21e6246544ad4c4082813db7511`.
- The enabled service is active with zero restarts. Migration/index checks,
  Caddy validation, serial Go tests and vet, local/public health/readiness,
  Swagger, raw OpenAPI, and the worker/status auth boundaries passed. The live
  contract contains 180 operations.

### 2026-08-14 — CPO charger live projection release deployed

- CPO charger list/detail responses now optionally include the committed
  `live` projection: charger connection state/freshness and connector
  availability/freshness. Missing projection rows remain non-fatal, and these
  reads never synchronously call HAL.
- CPO charger list uses one bounded `liveops.GetChargerDetails` batch read for
  the page instead of one projection query set per charger. OpenAPI and the
  human administrative contract describe the optional field.
- Revision `7350887` is active on the development VPS. No migration was
  required; migration 38 remains the latest applied migration. The installed
  binary SHA-256 is `4e5b771835dce699c8198915225654b0e9979c6d38bf603373cbc2b6591c13ab`.
- The enabled service is active with zero restarts on `127.0.0.1:18080`.
  Caddy validation, focused CPO/liveops/customer tests, serial Go tests and
  vet, route/OpenAPI parity, local/public health/readiness, Swagger, raw
  OpenAPI, and the protected CPO-route boundary passed. The live contract has
  180 operations.
- `pwsh` documentation verification and disposable PostgreSQL lifecycle tests
  remain unavailable on this host; full HAL/virtual-charger acceptance remains
  deferred pending its dedicated environment.

### 2026-08-13 — CPO charging-session read release deployed

- CPO `ADMIN` now has tenant-scoped `GET /api/v1/cpo/charging-sessions` list
  and `GET /api/v1/cpo/charging-sessions/{session_id}` detail routes. The
  list uses bounded descending `(created_at, id)` pagination and validates
  lifecycle status and UUID filters; detail and list never query HAL or expose
  another CPO's rows.
- The OpenAPI contract includes both operations and the `SessionStatus` enum;
  the human administrative API contract and CPO frontend handoff are updated.
- Revision `4cb1edd` is active on the development VPS. No migration was
  required; migration 38 remains the latest applied migration. The installed
  binary SHA-256 is `bba0cdc3305c16e339dc4e6e8b7e55624e5fcb835c2c8ea16755d98921f5e91b`.
- The enabled service is active with zero restarts on `127.0.0.1:18080`.
  Caddy validation, serial Go tests and vet, route/OpenAPI parity, local/public
  health/readiness, Swagger, raw OpenAPI, and the unauthenticated new-route
  boundary passed. The live contract contains 180 operations.
- `pwsh` documentation verification and disposable PostgreSQL lifecycle tests
  remain unavailable on this host; full HAL/virtual-charger acceptance remains
  deferred pending its dedicated environment.

### 2026-08-13 — Commercial tax and hub-prerequisite correction deployed

- Migration `000038_separate_tariff_gst_and_require_charger_hub` refuses to
  discard any non-null legacy `tariffs.gst_id`, then removes tariff GST
  ownership. It normalizes hubless chargers to hidden/inactive and adds checks
  preventing active or customer-visible hubless rows.
- Customer price and charging-start admission select tariff by
  `USERGROUP > CHARGER > HUB`, then independently load the active same-CPO GST
  assigned to the hub. Start snapshots retain `hub_id`, `gst_id`, and all GST
  rates; completion calculates base plus SGST+CGST+IGST from that snapshot.
- Zero tariffs and zero GST rates are valid. New starts still require a
  positive wallet balance; a free tariff creates a zero hold and derives a
  positive HAL energy limit from connector capacity and the existing maximum
  duration.
- Migration 38 is applied on the development VPS and revision `ebb57fb` is
  active. Tariff GST ownership is removed, hubless chargers are hidden and
  inactive, and database checks prevent active or customer-visible hubless
  rows. The 178-operation contract and public/local health/docs routes are
  healthy.

### 2026-08-13 — Tariff-targeting correction deployed

- Deployed source contains migration
  `000037_correct_tariff_targeting`. It normalizes each legacy tariff to one
  target using `usergroup > charger > hub` precedence, makes target columns
  individually nullable, makes `assigned_to` non-null, and enforces both
  exactly one target and target/assignment agreement. Charger targets use a
  same-CPO `(cpo_id, charger_id)` foreign key and can therefore be independent
  of a hub. The guarded down migration refuses non-hub targets rather than
  deleting data or inventing a hub relationship.
- CPO tariff create/list/get/update routes derive the immutable target solely
  from their nested hub, charger, or user-group URL. JSON target IDs are
  rejected, PATCH cannot move scope, and each scoped query filters its matching
  `assigned_to` value.
- Hub price, charger price, and new charging-start snapshots use the same
  server-side effective-tariff selector. Charger context resolves
  `USERGROUP > CHARGER > HUB`; hub-only context has no charger candidate.
- User App charger visibility remains the conjunction of charger
  `customer_visibility` and attached hub `customer_visible`. The image route
  now reuses that same published-charger check; discovery, locations, hub
  projections, direct detail, price, favorite reads/mutations, and new start
  admission retain it. Existing owned session history/detail/stop access is
  intentionally unaffected.
- Migration 37 was applied after a mode-0600 custom-format backup. The live
  database had zero tariff rows before migration, so no rows required
  normalization. No HAL/OCPP, legacy CMS, QR, wallet-settlement, or
  session-ownership contract changed.
- Clean runtime revision `a9fc32b` is active with 178 OpenAPI operations. The
  installed binary SHA-256 is
  `0b8c57d7991511e55d9d9200f57961b692ff5acd330968cd9345bbeb517884a1` and
  embeds `a9fc32b5517676fa3178689d02b87f529ddd0c79` with
  `vcs.modified=false`.
- The enabled service is active with zero restarts on `127.0.0.1:18080`.
  Migration/constraint checks, Caddy validation, loopback and public HTTPS
  health/readiness, Swagger, raw OpenAPI, 178-operation parity, protected
  tariff/customer-price boundaries, and the post-rehost journal scan passed.
- The pre-migration database dump is retained at
  `/tmp/devevcmsnewdb-pre-a9fc32b.dump` with mode `0600` and SHA-256
  `1ecec3a7e73b3417fa303239d0938af3f9991a40c1b016317724aea909515e1a`.

### 2026-08-13 — Charger customer-visibility release deployed

- Migration `000036_add_customer_visibility_to_chargers` is applied. The
  `chargers.customer_visibility` column is `BOOLEAN NOT NULL DEFAULT TRUE`.
- CPO ADMIN can publish or unpublish a charger through
  `PUT /api/v1/cpo/chargers/{charger_id}/customer-visibility`. User App
  discovery, direct lookup, charger pricing, and favorite reads/mutations now
  require both the charger gate and its hub's `customer_visible` gate.
  Existing customer sessions and active charging are unaffected.
- Clean runtime source revision `a9528c4` is active with 178 OpenAPI
  operations. The installed binary SHA-256 is
  `09b80b12865866b84e8a690bbaa2829257a036af8b2fee5c69fb3a7808a4b60c` and
  embeds `a9528c4843e706acaab7aff499f8234c3210d764` with
  `vcs.modified=false`.
- The enabled service is active with zero restarts on `127.0.0.1:18080`.
  Caddy validation, migration/column checks, loopback and public HTTPS health
  and readiness, Swagger, raw OpenAPI, 178-operation parity, the unauthenticated
  charger-visibility boundary, and the post-rehost journal scan passed.
- The pre-release database dump is retained at
  `/tmp/devevcmsnewdb-pre-18f261e.dump` with mode `0600` and SHA-256
  `234a98f1ba69e8aeb5fd35353c1ed97486a0e4c89b8a84e9509a7393fe97abbb`.

### 2026-08-13 — User App start-admission hardening release deployed

- The deployed release admits a new `POST /api/v1/app/charging-sessions`
  request only when the committed CMS `liveops` connector projection is
  `AVAILABLE` and `FRESH`. `CHARGING`, `FAULTED`, `UNAVAILABLE`, unknown,
  offline-parent, and stale projection states return the generic
  `409 connector_not_available` response before wallet holds, start intents,
  HAL command records, or HAL calls are created.
- An existing active intent owned by the same customer is replayed before the
  live admission gate; another customer receives the same generic conflict
  without identity details. The transaction takes a PostgreSQL `FOR UPDATE`
  lock on the connector and rechecks the active intent, so concurrent starts
  cannot both create a durable start path.
- No migration, route, or HAL/OCPP contract changed. Clean source revision
  `172bcd4` is active with 177 OpenAPI operations. The installed binary SHA-256
  is `ab53143ae0bb55d14e9256d77eb5bf3350ce1aed2c280236b41dc4ad80ea2238`.
- The enabled service is active with zero restarts; Caddy validation,
  migration/table checks, loopback/public health and readiness, Swagger, raw
  OpenAPI, the unauthenticated charging-start boundary, and the post-rehost
  journal scan passed.

### 2026-08-13 — State-aware GST-to-hub assignment release deployed

- GST assignment and replacement now enforce the hub/GST state relationship:
  same-state assignments require SGST and CGST and reject non-zero IGST;
  different-state assignments require IGST and reject non-zero SGST or CGST.
  Invalid combinations return `400 invalid_gst_for_hub`.
- Clean merge revision `4377383` is active on the development VPS without a
  database migration. The installed binary SHA-256 is
  `769b71782f47bb93c37e063dbab4ba8af34902b80ac8fb3150c21ac61c2fc5e0`.
- The service is enabled and active with zero restarts; Caddy validation,
  migration/table checks, loopback/public health and readiness, Swagger, raw
  OpenAPI, 177-operation parity, GST route boundaries, and the post-rehost
  journal scan passed.

### 2026-08-13 — User App charging history release deployed

- The deployed release adds `GET /api/v1/app/charging-sessions`, a bounded
  descending `(start_time, id)` history of only materialized sessions owned by
  the authenticated CPO-local customer. Each row has safe charger/hub/connector
  card data and only exposes final energy/amount after completion.
- Session detail now adds durable historical/financial fields, frozen tariff
  and tax snapshot projections, and the payment-to-wallet-debit relation only
  when it is consistently linked to the owned session. Runtime availability
  remains a CMS `liveops` projection and does not synchronously call HAL.
- `CHARGING_SESSION` operational events now use the actual materialized
  session UUID, so the User App can refetch the route named by `resource_id`.
  The canonical User App handoff documents one authenticated SSE stream,
  REST recovery, cursor replay, and why SSE remains pending in DevTools.
- No migration was required: the existing customer/session/start-time index
  supports the bounded history query. Clean source revision `87b8727` is active
  with 177 OpenAPI operations. The installed binary SHA-256 is
  `007d40cbd9eeda79392f7b1d546cc4d6e2bf336913212c6ce9f17dde4f9a6434`.
- The enabled service is active with zero restarts; loopback/public health and
  readiness, Swagger, raw OpenAPI, the unauthenticated history-route boundary,
  Caddy validation, migration/table checks, and the post-rehost journal scan
  passed.
- Physical charger and CMS-to-HAL-to-charger acceptance were not run and
  remain deferred by design.

### 2026-08-13 — HAL runtime model mapping release deployed

- The CMS GORM models now explicitly map `HALChargerRuntime` and
  `HALConnectorRuntime` to the singular migration tables
  `hal_charger_runtime` and `hal_connector_runtime`; focused model regression
  coverage protects the mapping.
- Clean source revision `0d50c09` is active on the development VPS. No new
  migration was required because the database already records migrations
  through thirty-five. The installed binary SHA-256 is
  `e3790854e68f7a3996d50a552e2f15ef6a95f644184e10389a95d513d64b24bf`.
- The service is enabled and active with zero restarts; loopback/public health
  and readiness, Swagger, raw OpenAPI, and the 176-operation contract passed.
  The HAL provider remains unconfigured and full dual-service acceptance is
  still pending.

### 2026-08-12 — CMS HAL operational capability release deployed

- Source now contains `halops` for CMS command/mapping/reconciliation and fact
  ingress, `liveops` for committed projection reads/freshness, and durable
  scoped operational-event persistence. CPO and Platform operational REST
  snapshots do not synchronously call HAL.
- Every full User App charger projection (list, hub detail, single detail, and
  favorites) now applies the same bounded committed live-state overlay; compact
  map locations intentionally remain charger name plus hub coordinates only.
  CMS administrative lifecycle is kept separate from runtime evidence and is a
  customer-safety gate for displayed availability.
- Migration thirty-five is applied on the development VPS; `hubs.gst_id` and
  its same-CPO foreign-key constraint are present alongside migrations
  thirty-three and thirty-four. Earlier revision `e831b32` was active with the
  176-operation contract and healthy local/public
  liveness, readiness, Swagger, and raw OpenAPI routes.
- Real PostgreSQL fact/mapping/SSE lifecycle, reconciliation recovery, and
  CMS-to-HAL-to-virtual-charger topology acceptance remain unverified pending
  the dedicated disposable database and provider test environment.
- The User App overlay/manual release is deployed. Its focused and broad source
  verification does not replace the pending disposable dual-service acceptance.
- Current source integration repair preserves those HAL/liveops capabilities,
  removes duplicated CPO operational response declarations and duplicate
  create-path mapping delivery, and aligns new charger `charger_id` and
  `ocpp_identity` values. Migration 34's nullable tariff assignment metadata is
  applied and remains unassigned by current tariff APIs.
- Current source separates connection freshness from meter freshness:
  `HAL_V1_CONNECTION_STALE_AFTER` controls durable HAL connection evidence,
  while meter data remains governed by `HAL_V1_METER_STALE_AFTER`. A fresh
  parent connection makes the latest accepted connector status live; stale,
  offline, or unknown parent evidence always makes it unavailable/stale.

The repository began as an empty file scaffold. The implemented foundation now
provides:

- a compilable Go service;
- PostgreSQL connectivity and versioned migration execution;
- process-liveness and database-readiness endpoints;
- always-on JSON HTTP completion logging with server-generated request IDs,
  matched route templates, result/latency/size fields, safe authenticated
  identifiers, handled API error codes, safe correlated panic stack
  diagnostics, and explicit secret/content exclusion;
- optional `LOG_LEVEL=DEBUG` request-start and handled-error
  component/type diagnostics under the same server request ID;
- global identities;
- separate platform-superadmin records;
- CPO tenant organizations;
- CPO membership persistence with active staff-role identity; CPO routes use
  explicit capabilities as endpoint authority while roles supply defaults;
  support and notifications retain their separately documented boundaries;
- tenant-scoped, credential-owning customer accounts;
- user settings and tenant customer groups;
- hubs, chargers, connectors, favorites, and group access links;
- GST profiles and tariffs;
- wallets, wallet transactions, charging sessions, and wallet payments;
- platform and tenant audit logs;
- matching up and down migrations plus an explicit rollback operation;
- environment-only, concurrency-safe, idempotent initial-superadmin bootstrap;
- Argon2id passwords and bounded login lockout/rate limits;
- mandatory email OTP for platform and CPO administrative login;
- signed-then-encrypted access JWTs and rotating opaque refresh tokens;
- durable sessions with list/current/all/specific revocation APIs;
- enumeration-safe password recovery and authenticated password change;
  eligible reset mail delivers the recovery ID, code, and expiry required by
  the reset handler while the forgot response remains generic, and every OTP
  producer uses the canonical complete mail payload without a lossy wrapper;
- trusted principal, user ID, CPO ID, platform, and CPO-role helpers;
- encrypted PostgreSQL mail outbox with a retrying, encrypted-transport SMTP
  worker;
- write-only encrypted Razorpay credentials for CPO admins;
- platform-only CPO create, searchable/filterable/cursor list, inspect, profile,
  reasoned activate/suspend, and app-ID APIs;
- required GSTIN plus complete address fields for CPO creation/profile
  replacement, backed by database constraints and normalized GSTIN uniqueness;
- authenticated platform slug-availability lookup for responsive FE validation,
  with final creation/database uniqueness remaining authoritative;
- constraint-aware platform CPO conflict responses that distinguish slug,
  GSTIN, app ID, administrator identity, membership, and primary-admin races;
- durable current lifecycle reason, actor, and transition time;
- one durable primary administrator per provisioned CPO, with safe visibility,
  replacement/restoration, credential-free onboarding resend, and targeted CPO
  administrative-session revocation;
- manually controlled pending CPO creation with unique dummy app IDs;
- transactional first-CPO-admin creation or safe existing-identity attachment;
- encrypted welcome email with CPO ID, app ID, and generated temporary
  password for new identities;
- non-expiring temporary-password change enforcement and login reminders;
- current dummy/live app identity in CPO login, refresh, and `me` responses;
- `X-CPO-App-ID` enforcement on tenant business APIs without trusting it as
  tenant authority;
- CPO ADMIN identity-profile read/update for global full-name and phone fields;
- session-bound, read-only CPO registration/organization details without
  exposing internal Superadmin actor metadata or permitting tenant mutation;
- tenant-scoped bounded hub create/list/get/update, including required stored
  state in CPO hub requests and response projections;
- atomic CMS charger/connector registration with server-generated charger UUID,
  public charger ID, OCPP mapping identity, and connector UUIDs;
- bounded charger listing, detail/update, connector update, and dependency-safe
  charger deletion;
- exact-decimal, bounded GST and tariff create/list/get/update with cross-CPO
  relationship rejection and INR defaulting;
- Hostinger implicit-TLS configuration on `smtp.hostinger.com:465`, with
  startup rejection of plaintext or ambiguous SMTP modes;
- registered educational, integration, API, internal-message, and
  configuration documentation under `docs/`;
- a canonical CPO backend AI-agent handoff covering current capability,
  ownership, tenant/HAL boundaries, remaining dependency order, slice
  execution, verification, and handoff requirements;
- a canonical SuperAdmin frontend handoff covering the 66-operation current
  platform integration surface, TypeScript contracts, auth/token state, CPO
  workflows, governance, security, mail, notifications, overview/status,
  audit/workers, SSE/replay, error UX, security, verification, and explicit
  deployment gaps;
- canonical OpenAPI 3.1 for all 135 current source-tree business/health
  operations;
- embedded same-origin Swagger UI at `/docs/` and raw OpenAPI at
  `/openapi.yaml`;
- CPO Swagger operations organized into Account & Notifications, Network,
  Pricing & Tax, and Integrations sections without mixing platform or User App
  operations;
- `API_DOCS_ENABLED` registration control for both documentation surfaces,
  defaulting on for compatibility and returning `404` when disabled;
- bidirectional verification that Gin and OpenAPI expose the same operation
  set;
- public CPO-scoped customer signup start, verify, and resend APIs;
- durable signup challenges with hashed pending passwords and HMAC-protected
  OTPs;
- transactional CPO-local customer-account and zero-balance INR wallet
  creation without a global administrative user;
- dedicated customer challenge, session, and refresh-token tables bound to one
  customer and CPO without a staff role or global-user foreign key;
- customer password-plus-mail-OTP login, signed/encrypted access tokens, and
  rotating/reuse-detecting refresh tokens;
- app-user `me`, customer-scoped session listing/revocation/logout, CPO-local
  password reset/change, and eligible-recipient recovery-ID/code delivery;
- authenticated customer self-service profile updates through
  `PATCH /api/v1/app/profile`, with omitted-versus-null phone semantics,
  canonical user projection responses, and CPO-scoped field-name-only audit
  evidence;
- `hubs.manage`-controlled default-false hub publication through
  `customer_visible`, plus authenticated customer-safe published network
  discovery for hubs, attached chargers, and connectors; the hub
  `open_24_hours` and charger `twenty_four_seven_open_status` values are
  separate, connector total capacity and static CMS administrative statuses are
  returned separately from HAL-owned live availability, which remains
  `UNKNOWN`;
- customer-owned favorite list and idempotent add/remove APIs over published
  hubs and attached chargers, with unpublish-safe reads and CPO/customer
  composite ownership;
- authenticated informational hub and charger price resolution using active
  effective tariffs, active GST projections, explicit `AVAILABLE`/
  `UNAVAILABLE` states, and User Tariff > charger tariff > hub tariff
  precedence;
- authenticated User App charger search/filter and bounded near-me reads over
  published hubs, with safe hub, display/category/parking charger, and
  connector projections; DB-backed status and connector total capacity; an
  authenticated charger-image route keyed by public charger ID; and explicit
  UNKNOWN live availability;
- an authenticated compact User App charger-location list over the same
  published-hub scope and optional charger filters, returning each map marker
  as only `charger_name`, attached-hub `latitude`, and attached-hub
  `longitude`;
- authenticated CPO/customer-scoped wallet balance and keyset-paginated wallet
  history reads using exact decimal projections;
- User App Razorpay recharge order creation and captured-payment verification
  through the existing encrypted CPO integration credentials, with migration
  twenty-two durable recharge orders, provider payment attempts, future-refund
  records, provider snapshots, signature evidence, and atomic wallet-credit
  ledger linkage; no CPO/Superadmin payment APIs, refund execution, webhook,
  settlement reconciliation, RFID, or HAL integration;
- trusted backend current-principal, customer, CPO, and app-ID helpers, with
  `CurrentUserID` retained as a customer-ID compatibility alias;
- a separated User App route topology: credential/session operations remain
  under `/api/v1/app/auth`, while authenticated app resources (`me`, profile,
  discovery, favorites, pricing, wallet, and recharge) are under
  `/api/v1/app`;
- environment-controlled permissive CORS middleware and a current development
  configuration that listens on all IPv4 interfaces for access from other
  machines;
- durable platform event replay, authenticated SSE, filtered audit query, and
  registered worker-health/readiness APIs;
- complete source-tree platform-superadmin governance: administrator
  invite/grant/activate/deactivate with last-active-admin protection;
- source-tree security operations for locked identities, reasoned unlock,
  security-event visibility, and scoped user session revocation;
- source-tree safe mail-job administration for metadata listing, retry/cancel,
  metrics, stale-job reconciliation, and audited bounded retention;
- source-tree platform/CPO announcements with immutable audience snapshots and
  durable recipient notifications, including recipient-owned read state;
- source-tree bounded platform overview and service-status aggregates;
- platform-superadmin-only manual subscription plans, immutable published plan
  versions, explicit CPO issue/renew/change/status commands, and idempotent
  transition history;
- server-generated subscription UUIDs/version numbers/timestamps plus atomic
  audit/platform-event records; no client supplies those values;
- no provider, checkout, invoice/payment API, webhook, subscription mail,
  automatic renewal, scheduled transition, or lifecycle worker; CPO
  activation/suspension remains independent from subscription state;
- a reversible migration-nine retirement boundary that preserves the removed
  prototype tables in `retired_commercial` and disables their worker records;
- an active VPS deployment at `dev-evcmsnew.transev.site`, with Caddy proxying
  to the loopback-only listener `127.0.0.1:18080`;
- an enabled and active `evcmsnew-dev.service`, ignored mode-0600 deployment
  environment, compiled binary layout, and `rehost-evcmsnew` interactive
  handler;
- the additive PostgreSQL database `devevcmsnewdb`, owned by `postgres`.

The active development VPS runs source revision `0ad2de7`, with migrations
through forty-three recorded and the deployed 182-operation contract. Migration
twenty-nine adds nullable `tariff_type`, `price_type`, and `units` metadata to
tenant tariffs; omitted values remain null-safe for existing and newly created
tariffs. The SuperAdmin administrator-list query explicitly binds the platform
administrator model. The User App can serve an authenticated published charger's allowed image through
its relative `charger_image_url`, without exposing the stored upload path.
The CPO charger response also exposes a read-only `assigned` projection that
matches hub attachment, and Swagger groups CPO operations by account/network,
pricing/tax, and integration responsibilities.
The CPO ADMIN customer directory is read-only and tenant-scoped, and CPO
charger projections expose `hub_name` when assigned. Connector capacity is
stored and exposed as `connector_total_capacity` in CPO create/update requests
and response projections, as well as User App connector projections.
User App hub summaries expose `open_24_hours`; charger projections separately
expose the charger's `twenty_four_seven_open_status` and the attached hub's
`hub_open_24_hours`.
CPO hub, charger, and user-group tariff routes are tenant-scoped; CPO user-group
CRUD is protected by the same administrative authorization boundary.
The user-group member-assignment and member-removal operations are
tenant-scoped and idempotent; membership changes record an audit action, and
the user-group detail response exposes safe `members` customer projections;
customer projections expose `usergroup_assigned`. The tenant-scoped CPO settings
API exposes invoice-note and invoice-logo metadata through authenticated GET,
POST, and PUT routes; migration thirty stores one settings row per CPO. A
separate authenticated CPO invoice-logo retrieval route streams only that
tenant's stored logo and does not disclose a filesystem path. A
GST profile now has a required API-level `state` value; migration thirty-one
adds the nullable durable column and permits legacy GST-rate values to remain
null. New GST creation continues to require all three validated rates. A
required hub `state` value is now stored by migration thirty-two and returned by
the CPO hub contracts. The authenticated User App charger-location route is
also live and returns only the documented compact map-marker projection. A
connected platform realtime SSE
client can make graceful shutdown reach its bounded timeout during rehost;
systemd restarts the service automatically and health checks then pass.
Migration
twenty-seven replaces legacy charger/connector protocol-style states with the
static CMS administrative values `ACTIVE`, `INACTIVE`, `SUSPENDED`,
`UNDERMAINTENANCE`, and `DECOMMISSIONED`. CPO GSTIN and address
identity are database-required, authenticated platform clients can use the
advisory slug-availability operation, and known uniqueness races return
field- or relationship-specific conflict codes. The current database contains
two CPO records. Migration nine continues to preserve the
`retired_commercial` schema. Safe structured HTTP request logging is active;
the current development environment uses `LOG_LEVEL=DEBUG` for correlated
request-start and completion diagnostics.

Migration twenty-eight adds the CMS-owned charging start-intent, wallet-hold,
HAL-command, fact-receipt, charger-mapping, and charger/connector runtime
projection state. The current deployment exposes the first customer charging
start/stop/status and HAL fact-receiver routes, but its optional HAL v1 base URL
and credentials are unset, so customer charging remains unavailable until the
approved independent provider is configured.

Migration thirteen removes feature-key runtime behavior and is deployed.
Migration fourteen completes the deployed Superadmin control-plane surface.
Migration fifteen adds tariff effective-date fields and a tenant/scope-aware
PostgreSQL exclusion constraint and is deployed. The disposable PostgreSQL
lifecycle test remains unexecuted because no `TEST_DATABASE_URL` is configured.

The deployed contract has 129 operations: the added
`GET /api/v1/cpo/users/{user_id}` is a tenant-scoped staff-membership point
lookup, not a customer or staff directory. CPO-local customer accounts are not
reachable through it.

The deployed source has 129 operations. It includes
operations after adding customer self-service profile editing, published
network discovery, favorites, informational customer price reads, charger
search and wallet reads, and Razorpay recharge order/verification; the deployed binary remains at 113 until a
separately approved deployment. The deployed
source includes the `hubs.manage`-protected
`POST /api/v1/cpo/hubs/{hub_id}/chargers` hub attachment/reassignment command,
allows an independent charger to be created without `hub_id`, and adds
non-negative hub `sanction_load` plus the upgrade-time removal of the legacy
charger-hub `NOT NULL` in migration sixteen. These changes are deployed at
`782dd7b`; migration nineteen reconciles databases that had already recorded
the removed follow-up migrations so `chargers.hub_id` is nullable, and migration
twenty makes customer accounts CPO-local with dedicated authentication lineage,
and migrations 21–22 add customer-visible network discovery and Razorpay wallet
recharge ledger support. Migration twenty-six removes obsolete connector
current/voltage fields; connector capacity is represented by
`connector_total_capacity` in CPO create/update requests and response
projections, and in User App connector projections.

The deployed recovery flow fixes a recovery-specific OTP mapper defect that discarded
`challenge_id` before outbox validation and caused eligible administrative and
customer forgot-password transactions to roll back with `500 internal_error`.
Administrative and customer recovery now enqueue the complete canonical mail
payload before outbox validation.

CPO customer-app implementation requires `X-CPO-App-ID` on every
`/api/v1/app/...` request, including signup. The approved next user-work
plan retains that app-only header. Customer self-service name and phone
editing, published-station discovery, favorites, and informational tariff
display are implemented in source; HAL-dependent charging/billing work remains
planned.
CPO capability-protected routes remain owned by the CPO workstream.

The CMS/HAL transport, authenticated fact receiver, durable charging intent and
hold state, and customer charging start/stop/status routes are implemented and
deployed. Full HAL handshake, live charger state ingestion, virtual-charger
acceptance, restart/outage recovery, reconciliation worker, Razorpay
refund/webhook/settlement workflow, tenant commercial-management workflow,
staff-management workflow, and reporting behavior remain incomplete or
intentionally unsupported.

## Verification

- Go formatting completed.
- Safe request completion logging, secret/content exclusion, loopback-only
  forwarded-address trust, handled error correlation, authentication failure,
  safe recovered-panic diagnostics, stock request-dump suppression, and CORS
  request-ID exposure have focused test coverage.
- DEBUG request-start and handled-error diagnostics have focused mode,
  correlation, classification, and secret/content leak coverage.
- Complete direct OTP payloads and both administrative/customer recovery
  template validations have database-free regression coverage. The changed
  PostgreSQL recovery lifecycle was not run because no disposable
  `TEST_DATABASE_URL` was configured.
- Known CPO unique-constraint mappings and the unknown-constraint fallback have
  focused unit coverage; PostgreSQL lifecycle assertions now require the exact
  slug and GSTIN conflict codes.
- Required-field validation, slug normalization/authorization, migration
  content, and affected package tests passed for the source-tree change.
- Superadmin migration fourteen static coverage, input/privacy regression
  tests, and the affected package tests passed for the source-tree change.
- The 124-operation source OpenAPI and runtime route sets match; documentation
  contract verification passed.
- Source migration coverage verifies both sanctioned-load constraints and the
  upgrade/rollback guard for independent charger inventory. The targeted
  PostgreSQL hub-assignment lifecycle remains pending because no explicitly
  disposable `TEST_DATABASE_URL` is configured.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed. Stateful PostgreSQL lifecycle verification is
  intentionally deferred by the current workstream decision; no stateful
  result is claimed.
- Revision `6930189` was built from a clean worktree and rehosted without a
  new migration. The running systemd process matches the installed binary
  SHA-256 `82781f61691cd93061f5e0d866ee63ce109298d053e74060d4242c2c8a817efe`
  and embeds revision `693018920d5600592a5b3a8409ce2e7790e25f80`
  with `vcs.modified=false`. The service is active and enabled; loopback and
  public liveness/readiness passed; the live OpenAPI contains 152 operations;
  the live CPO user-group member detail projection and `usergroup_assigned`
  response fields are present; and the post-start warning scan passed.
  The PowerShell documentation verifier remains unavailable on this Ubuntu
  host.
- Revision `d368903` was built from a clean worktree and rehosted without a
  new migration. The installed binary SHA-256 is
  `bf9c5bdd7ac9205b45086a513b014bce1c371991a4a61ab23ff9541ee5f9eb07` and
  embeds revision `d368903a960c519293b8ea26a9415902eb381056` with
  `vcs.modified=false`. The enabled service is active with zero restarts;
  local/public health, Swagger, raw OpenAPI, the 161-operation contract, and
  protected settings/invoice-logo/GST/SuperAdmin route checks passed. The
  bounded SSE shutdown deadline occurred during stop and recovered; current
  systemd state is `Result=success`.
- Revision `3ca2c35` was built from the clean `main` worktree and rehosted after
  applying migration thirty-two. The installed candidate SHA-256 is
  `1771f4bee02ed7f5d270f9feacdad69ecb4c3de435103ea5d7c97e176ab9da12`, with
  rollback binary `builds/evcmsnew.pre-3ca2c35` and database dump
  `/tmp/devevcmsnewdb-pre-3ca2c35.dump`. The enabled service is active with
  zero restarts; loopback/public liveness and readiness, Swagger, raw OpenAPI,
  the 162-operation contract, and the unauthenticated protected charger-location
  check passed. The post-rehost journal had no error, panic, or fatal records.
  The pre-rehost process emitted a cached-plan result-type warning after the
  schema change; it was eliminated by the rehost and is not present in the new
  process logs. The PowerShell documentation verifier remains unavailable on
  this Ubuntu host.
- Revision `3f3a952` was built from the clean `main` worktree and rehosted
  after migration thirty-three. The installed binary SHA-256 is
  `c2d0198da5365c7afde37726d6b48e95504c48112bb5aa94e9601af09320ddbd`,
  embeds `3f3a9522f94b333231d21dea2366eb1cd2e8418d` with
  `vcs.modified=false`, and retains `builds/evcmsnew.pre-3f3a952` plus a
  mode-0600 PostgreSQL rollback dump. The enabled service is active with zero
  restarts; migration ledger/table/index checks, loopback/public health and
  readiness, Swagger, raw OpenAPI, the 172-operation contract, and HAL
  fact-ingress validation passed. No DNS, Caddy, or HAL provider configuration
  changed.
- Revision `27c01f3` was built from the clean `main` worktree and rehosted
  without a database migration. The installed binary SHA-256 is
  `8e530d531f53691edc1cfde8970a83bf105160aa4ab3dfdb64bf3ae0d82191cb`,
  embeds `27c01f3d82bc39498ad78f46ad369d90f5d7e7e0` with
  `vcs.modified=false`, and retains `builds/evcmsnew.pre-27c01f3`. The enabled
  service is active with zero restarts; loopback/public health and readiness,
  Swagger, raw OpenAPI, the unchanged 172-operation contract, and the deployed
  User App full-response live-overlay descriptions passed. No DNS, Caddy, HAL
  provider configuration, or schema changed, and post-restart warnings were
  absent.
- Revision `2e8fdb3` was built from the clean `main` worktree and rehosted after
  migration thirty-four. The installed binary SHA-256 is
  `95bbae5bf45576f452f4fd2eb42ce0542e29b4c943eae04ba7ee1ecf21618255`,
  embeds `2e8fdb33abf105a58eabe894e43ab487ee7cc9be` with
  `vcs.modified=false`, and retains `builds/evcmsnew.pre-2e8fdb3` plus a
  mode-0600 PostgreSQL rollback dump. The enabled service is active with zero
  restarts; migration ledger/type/column checks, loopback/public health and
  readiness, Swagger, raw OpenAPI, the 172-operation contract, and the
  post-rehost warning scan passed. No DNS, Caddy, or HAL provider configuration
  changed.
- Revision `e831b32` was built from the clean `main` worktree and rehosted after
  migration thirty-five. The installed binary SHA-256 is
  `10e111c90f4082badeb13155e654770eec6810a39627eb075d7e8c5aa72b91d9`,
  embeds `e831b32be0edd0eaa76337ddc75cf86c1c7dbc01` with
  `vcs.modified=false`, and retains `builds/evcmsnew.pre-e831b32be0ed` plus a
  mode-0600 PostgreSQL rollback dump. The enabled service is active with zero
  restarts; migration ledger/column/constraint checks, local/public health and
  readiness, Swagger, raw OpenAPI, the 176-operation contract, and all four
  GST-hub unauthenticated route boundaries passed. The old process emitted the
  expected stop-time exit warning during rehost; the current process has no
  error or fatal records. No DNS, Caddy, or HAL provider configuration changed.
- Revision `0d50c09` was built from the clean `main` worktree after the
  HAL-runtime GORM table-name correction and rehosted without a migration. The
  installed binary SHA-256 is
  `e3790854e68f7a3996d50a552e2f15ef6a95f644184e10389a95d513d64b24bf`, embeds
  `0d50c096ecffef368d7c64208dcf4d6391ff0f38` with `vcs.modified=false`, and
  retains `builds/evcmsnew.pre-0d50c09` as the prior-binary rollback artifact.
  The enabled service is active with zero restarts; migration 35/table checks,
  local/public health and readiness, Caddy validation, Swagger, raw OpenAPI,
  the 176-operation contract, and the post-rehost journal scan passed. No DNS,
  Caddy, HAL provider, or database migration changed.
- Revision `87b8727` was built from the clean `main` worktree and rehosted
  without a migration for the User App charging-history release. The installed
  binary SHA-256 is
  `007d40cbd9eeda79392f7b1d546cc4d6e2bf336913212c6ce9f17dde4f9a6434`, embeds
  `87b87271fb6d517a9f9d80e7a18b69da13db4b7e` with `vcs.modified=false`, and
  retains `builds/evcmsnew.pre-87b8727` as the prior-binary rollback artifact.
  The enabled service is active with zero restarts; migration 35/table checks,
  local/public health and readiness, Caddy validation, Swagger, raw OpenAPI,
  the 177-operation contract, the unauthenticated history-route boundary, and
  the post-rehost journal scan passed. No DNS, Caddy, HAL provider, or database
  migration changed.
- Merge revision `4377383` was built from the clean `main` worktree and rehosted
  without a migration for state-aware GST-to-hub validation. The installed
  binary SHA-256 is
  `769b71782f47bb93c37e063dbab4ba8af34902b80ac8fb3150c21ac61c2fc5e0`, embeds
  `437738308d9d27fef2261518161a09075407caeb` with `vcs.modified=false`, and
  retains `builds/evcmsnew.pre-4377383` as the prior-binary rollback artifact.
  The enabled service is active with zero restarts; migration/table checks,
  local/public health and readiness, Caddy validation, Swagger, raw OpenAPI,
  the 177-operation contract, GST route boundaries, and the post-rehost journal
  scan passed. No DNS, Caddy, HAL provider, or database migration changed.
- Runtime revision `a9528c4` was built from the clean `main` worktree after
  migration thirty-six was applied. The installed binary SHA-256 is
  `09b80b12865866b84e8a690bbaa2829257a036af8b2fee5c69fb3a7808a4b60c`, embeds
  `a9528c4843e706acaab7aff499f8234c3210d764` with `vcs.modified=false`, and
  retains `builds/evcmsnew.pre-a9528c4` as the prior-binary rollback artifact.
  The enabled service is active with zero restarts; migration/column checks,
  Caddy validation, loopback/public health and readiness, Swagger, raw
  OpenAPI, the 178-operation contract, the unauthenticated charger-visibility
  boundary, and the post-rehost journal scan passed.
- Revision `172bcd4` was built from the clean `main` worktree and rehosted
  without a migration for User App charging-start admission hardening. The
  installed binary SHA-256 is
  `ab53143ae0bb55d14e9256d77eb5bf3350ce1aed2c280236b41dc4ad80ea2238`, embeds
  `172bcd4c9b9aaed3671983f6b98cd2018c1ac720` with `vcs.modified=false`, and
  retains `builds/evcmsnew.pre-172bcd4` as the prior-binary rollback artifact.
  The enabled service is active with zero restarts; migration/table checks,
  local/public health and readiness, Caddy validation, Swagger, raw OpenAPI,
  the 177-operation contract, the unauthenticated charging-start boundary, and
  the post-rehost journal scan passed. No DNS, Caddy, HAL provider, or database
  migration changed.
- Revision `a76d6ae` was built from a clean worktree and rehosted after
  migration thirty-one was applied. The installed binary SHA-256 is
  `0a3d397464dae13ef15b090225b4ca38fb1b4dfff946bf0de7d77cb9a5d3ebc0` and
  embeds revision `a76d6ae09dde7727661238f55e1b2ff5007394d0` with
  `vcs.modified=false`. The enabled service is active with zero restarts;
  local/public health, Swagger, raw OpenAPI, the 160-operation contract, and
  protected GST/settings/SuperAdmin route checks passed. The bounded SSE
  shutdown deadline occurred during stop and recovered; current systemd state
  is `Result=success`.
- Revision `e5fd599` was built from a clean worktree and rehosted after
  migration thirty was applied. The installed binary SHA-256 is
  `b6c251b9a65343db5eedbff3fee293678f4ac51cf401975b1d77380b1e47ef84` and
  embeds revision `e5fd599790b4fd9983ba055fc03b10637c2ad674` with
  `vcs.modified=false`. The enabled service is active with zero restarts;
  local/public health, Swagger, raw OpenAPI, the 160-operation contract, and
  protected settings/SuperAdmin route checks passed. The bounded SSE shutdown
  deadline occurred during stop and recovered; current systemd state is
  `Result=success`.
- Revision `2550cf7` was built from a clean worktree and rehosted after
  migration twenty-nine was confirmed current. The installed binary SHA-256
  is `5ebd7181ecae90a27787791c7c6f9786a3150fa968b3eb1d57bafa910c2418fa` and
  embeds revision `2550cf79fa6a9b84f3e30b0dca4101b8f0659574` with
  `vcs.modified=false`. The enabled service is active; local/public health,
  Swagger, raw OpenAPI, the 157-operation contract, and protected-route checks
  passed. The stop phase encountered the documented bounded SSE shutdown
  deadline and recovered; current systemd state is `Result=success` with
  `NRestarts=0`.
- Revision `f7e7227` was built from a clean worktree and rehosted after a
  mode-0600 PostgreSQL rollback dump and migration twenty-eight. The running
  systemd process matches the installed binary SHA-256
  `ab221e733317a832ea6b5bac60f5dcdc99c5a41d1f71d8edc2153b1c3161e957` and
  embeds revision `f7e722765809d0126b1ad8e84ba4ebb88e65f1d2` with
  `vcs.modified=false`. The service is active and enabled; loopback/public
  liveness/readiness, Swagger, raw OpenAPI, the 157-operation live contract,
  migration 28, protected charging routes, and the post-start warning scan
  passed. HAL provider credentials remain intentionally unset, so end-to-end
  virtual-charger acceptance is not claimed.
- Revision `79683f0` was built cleanly and rehosted without a new migration.
  The installed binary, loopback-only listener, local/public liveness and
  readiness, live 137-operation Swagger/OpenAPI with grouped CPO operations,
  protected CPO customer-list route, and post-start journal passed.
- Revision `9ccdff2` was built cleanly and rehosted after confirming migration
  twenty-seven and its replacement status constraints were already applied.
  The installed binary, loopback-only listener, local/public liveness and
  readiness, live 132-operation Swagger/OpenAPI, status and hub-publication
  operations, and post-start journal passed. The charger-create defaults now
  use the migration-27 values (`INACTIVE` charger, `ACTIVE` connector), so new
  inventory satisfies the live database constraint.
- Revision `782dd7b` was built cleanly and rehosted after a validated mode-0600
  rollback dump and migration twenty-six. The installed identity, loopback-only
  listener, loopback/public readiness, live 129-operation Swagger/OpenAPI,
  request-ID header, protected CPO routes, nullable charger hub assignment,
  required workers, and absence of startup errors or panics passed. Charger
  vendor and model persistence columns are nullable; charger create/update and
  customer network projections now preserve omitted metadata as null. Charger
  and customer-network part-time fields use `twenty_four_seven_open_status`.
  User App resources are served under `/api/v1/app`, while credentials and
  sessions remain under `/api/v1/app/auth`; the retired resource URLs return
  `404`. Charger-level `total_capacity`, connector current, and connector
  voltage fields are removed; connector-level capacity remains part of the
  connector contract.
- Revision `396bae5` was built cleanly and rehosted after a validated
  mode-0600 rollback dump and migration fourteen. The installed identity,
  loopback-only listener, loopback/public readiness, live 110-operation
  Swagger/OpenAPI, zero-restart service state, new authority columns and
  announcement/notification tables, and absence of startup errors or panics
  passed.
- `git diff --check` passed.
- Revision `9b508ef` was built cleanly and rehosted after a validated
  mode-0600 rollback dump and migration thirteen. The installed identity,
  loopback-only listener, loopback/public readiness, live 87-operation
  Swagger/OpenAPI, zero-restart service state, preserved feature-key rows in
  `retired_commercial`, and absence of startup errors or panics passed.
- Revision `d27e599` was built cleanly and rehosted without a migration. The
  installed identity, loopback-only listener, loopback/public readiness, live
  70-operation Swagger/OpenAPI, zero-restart service state, DEBUG request-start
  and completion records, and absence of startup errors or panics passed.
- Revision `9760523` was built cleanly and rehosted with migration eleven
  already current. The installed hash, loopback/public readiness, live
  70-operation Swagger/OpenAPI with field-specific conflict codes, protected
  and retired routes, required workers, and journal passed.
- Revision `afd90f5` was built cleanly, migrated through version eleven, and
  rehosted. The installed hash, loopback/public readiness, live 70-operation
  Swagger/OpenAPI, protected slug route, retired routes, required workers,
  migration constraints, and journal passed.
- Revision `1cec3f3` was built cleanly and rehosted. The installed hash,
  loopback/public liveness and readiness, live Swagger/OpenAPI, protected and
  retired routes, required workers, migration ledger, and journal passed.
- The read-only CPO organization projection, privileged-field omission,
  protected route, 69-operation OpenAPI parity, documentation contract, and
  complete CPO organization/profile/network/pricing lifecycle passed. The
  lifecycle used an explicitly created and removed disposable PostgreSQL
  database.
- The embedded up migration created all 21 domain tables in a disposable local
  PostgreSQL 17 database.
- Reapplying up was idempotent and retained one migration version.
- The matching down migration removed all domain tables and retained only the
  migration ledger.
- The auth migration rolled down, up, and idempotently up in PostgreSQL 17.
- Bootstrap twice retained one platform admin and did not overwrite its
  password.
- Platform and CPO admin email-OTP login passed using encrypted outbox payloads.
- Encrypted access-token validation, refresh rotation, reuse-triggered session
  revocation, and password reset handling passed. Updated lifecycle coverage
  obtains both recovery ID and code from the encrypted recipient mail payload
  rather than internal challenge storage; that changed PostgreSQL lifecycle was
  not run in this slice because no disposable `TEST_DATABASE_URL` was set.
- The mail worker claimed, decrypted, delivered through a test sender, and
  completed a durable job.
- Hostinger implicit-TLS configuration loaded successfully and the SMTP sender
  construction tests passed without exposing a real mailbox password.
- The required documentation, administrative route coverage, and removed SMTP
  configuration residue checks passed.
- OpenAPI parsing, semantic validation, all 40 Gin/OpenAPI operation matches,
  and docs/raw-spec HTTP smoke tests passed.
- Documentation routes were absent while ordinary health routes remained
  available with `API_DOCS_ENABLED=false`.
- Razorpay secrets remained encrypted in storage, resolved only internally for
  the correct CPO, and were unavailable to a platform principal.
- Route registration, unauthenticated rejection, request validation, and
  no-store behavior passed.
- The CPO provisioning migration rolled down, up, and idempotently up in
  PostgreSQL 17.
- CPO creation without commercial prerequisites, encrypted temporary-password
  delivery,
  concurrent and sequential identity reuse, repeated-login reminders,
  business-API password gate, password change, activation, dummy-to-live
  app-ID rotation, and suspension passed in PostgreSQL lifecycle tests.
- The customer-signup migration rolled down, up, and idempotently up in
  PostgreSQL 17.
- The superseded global-customer lifecycle previously passed PostgreSQL tests.
  Migration twenty and the current CPO-local lifecycle have source/static/full
  Go coverage but still require an explicitly disposable `TEST_DATABASE_URL`.
- The customer-authentication migration rolled down, up, and idempotently up
  in PostgreSQL 17.
- Customer login OTP, encrypted access validation, `me`, refresh rotation and
  reuse revocation, customer-scoped session listing/revocation/logout, and
  password reset/change are implemented on dedicated CPO-local auth tables.
  Their updated lifecycle compiled but was not executed against PostgreSQL in
  this slice because no disposable `TEST_DATABASE_URL` was set.
- Permissive CORS preflight behavior and the disabled-CORS path passed focused
  route tests; authentication and authorization remain active in either mode.
- Platform operations compile; model parsing, migration discovery/pairing, mail
  rendering, route protection, and the retained realtime/worker contracts pass.
- The current 49 Gin/OpenAPI operations match bidirectionally; all retired
  commercial routes return `404` and are absent from OpenAPI.
- Migration ten applied and rolled back against disposable loopback PostgreSQL
  17. Its PostgreSQL CPO lifecycle test passed creation correlation,
  primary-admin uniqueness, search/cursor listing, profile replacement,
  reasoned idempotent activation, previous-primary session/refresh revocation,
  credential-free onboarding resend, and platform-session isolation.
- Migration nine applied against disposable loopback PostgreSQL 17, archived
  commercial tables without losing a preserved row, blocked pending commercial
  mail, disabled retired worker records, rolled down with data intact, and
  restored worker requirements.
- PostgreSQL execution of migrations six through eight passed against the
  PostgreSQL 18.4 development deployment before the commercial surface was
  retired.
- All ten forward migrations are recorded in the active deployment's
  `schema_migrations`; the runtime `public` schema contains 31 tables and
  `retired_commercial` contains the 11 archived prototype tables.
- Migration ten added CPO lifecycle evidence, primary-administrator
  designation, and safe mail-correlation fields without changing existing row
  counts; the pre-migration database contained no CPO or membership rows.
- The configured platform superadmin remains bootstrapped exactly once.
- Loopback and public HTTPS liveness and database-readiness checks passed.
- Swagger UI and raw OpenAPI return `200`; the live OpenAPI contains all 49
  operations, while unauthenticated requests to the platform-managed CPO
  profile and primary-administrator APIs return `401`.
- Migration nine marked subscription lifecycle and billing maintenance workers
  disabled and non-required. Platform maintenance and mail outbox remain the
  required runtime workers.
- Readiness is evaluated per required worker name and succeeds when at least
  one instance is fresh and healthy. Stale records from replaced processes no
  longer degrade a healthy replacement.
- A real platform-login request returned `202`, and its encrypted
  `LOGIN_OTP` outbox job reached `SENT` on the first attempt through the
  configured Hostinger SMTP account.

## Current Access Model

- `users` represent administrative login identities for platform and CPO staff.
- `platform_admins` explicitly grant platform-superadmin authority.
- `cpos` represent tenant/customer organizations.
- `cpo_memberships` store a fixed role inside one CPO; callable endpoint
  authority is the route capability evaluated with the active membership and
  any override.
- `customers` are CPO-local app-user accounts and own email, password, profile,
  verification, lockout, and login timestamps.
- Customer-auth outbox jobs are correlated to their owning CPO without
  pretending that the customer has an administrative `users` identity.

The full mapping from the supplied schema is recorded in `docs/SCHEMA.md`.

The same administrative identity may belong to multiple CPOs as staff. App
customers are separate: the same email can register independently under
multiple CPOs, with independent password/profile/session state. An active CPO
staff membership can establish a CPO session. Core CPO administration and
provider-integration routes currently require ADMIN; support and notification
routes use their explicitly scoped active-membership boundary.

An administrative session selects exactly one platform or CPO scope. Protected
requests revalidate the durable session and current authority. Tenant context
comes from that principal rather than a request header. Tenant business routes
also verify that `X-CPO-App-ID` equals the current dummy or live ID for that
same principal; the app ID never grants authority or changes scope.

## HAL Boundary

The HAL remains a separate service and database. It is not embedded in this
repository. Source now contains the first CMS v1 consumer: an authenticated HAL
client, separate fact bearer receiver, durable start-intent/hold/command/fact
receipt/mapping/runtime records, and customer start/stop/polling routes.

This is not yet a verified complete operational integration. Disposable CMS and
HAL PostgreSQL lifecycle tests, bounded reconciliation, and the required
loopback HAL plus virtual OCPP charger acceptance remain outstanding. Customer
discovery must continue to treat CMS administrative status separately from
HAL-owned live runtime state.

## Known Limitations

- Password-recovery emails queued before recovery-ID delivery was implemented
  contain only the OTP and expiry and cannot complete reset. Users must request
  a new email; no database challenge lookup is an approved client workflow.
- A successful CPO-creation response proves its encrypted onboarding job
  committed, not that SMTP sent it. Operators must use primary-admin delivery
  status; only a newly created global identity receives a temporary password.
- Only the initial administrator profile and network/GST/tariff subset has
  handlers. Customer directory, charging, wallet mutation, recharge, payments,
  reporting, and most other domain tables remain without business APIs;
  published read-only customer network discovery, favorites, charger
  search/near-me, wallet balance/history, and informational tariff price reads
  are implemented.
- CPO staff invitation after the first admin and customer email/profile-change
  workflows are not implemented.
- Manual subscriptions are Superadmin-managed records; a CPO ADMIN has only a
  read-only current-subscription view. Feature keys and entitlement overrides
  are not defined. Platform invoices/payments and all provider or automatic
  lifecycle behavior remain unsupported, and CPO access is manual and
  independent.
- Automatic encryption-key rotation is not implemented; data must be
  re-encrypted before an old key is removed.
- SMTP delivery logic is implemented, worker-tested, and verified through one
  real Hostinger platform-login OTP delivery. The mailbox password remains only
  in the ignored deployment environment.
- No generated frontend SDK exists yet; consumers use the reviewed OpenAPI
  contract directly.
- Migration twenty's disposable PostgreSQL lifecycle coverage is intentionally
  deferred by decision. The live development deployment is current on
  migration twenty and the 113-operation contract; the two dormant feature-key
  tables are in
  `retired_commercial` while automatic lifecycle workers remain disabled.
