# WI-20260812-cms-hal-operational-capabilities

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (CMS/HAL boundary owner)
Started: 2026-08-12
Last updated: 2026-08-25

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — Charging lifecycle and HAL integration
Detailed-plan reference: `docs/integrations/ocpp-hal-boundary.md`
Issue/PR reference: None

## Outcome

Establish reusable CMS capabilities over HAL-derived operational truth and expose CPO, App, and Platform-safe live snapshots without direct HAL coupling.

## Scope

- HAL operations/reconciliation, shared fact ingress, CMS-projection live reads, inventory-owned pending mappings, CPO/App/Platform operational REST and SSE, durable operational-event publication, migration, OpenAPI, integration docs, and guarded development deployment.

## Non-goals

- HAL, legacy CMS, OCPP protocol, tariff/GST redesign, Superadmin charger control, and physical-charger acceptance.

## Claimed surfaces

- `src/halclient`, `src/halops`, `src/liveops`, `src/operationalrealtime`, `src/customerauth`, `src/cpo`, models/migrations, routes, OpenAPI, and HAL integration docs.
- HAL-to-CMS `transaction.started` failure classification, start-intent
  monotonicity, authoritative transaction lookup reconciliation, and User App
  truthful recovery after a failed fact projection. The counterpart HAL work
  item owns outbox delivery diagnostics.
- HAL command-response semantic validation, HAL command identity persistence,
  and authoritative-start repair of the invalid legacy zero UUID sentinel.
- User App full charger list, hub detail, single detail, and favorites; compact
  map locations remain explicitly out of scope.

## Dependencies and blockers

- Read-only HAL provider `21836e5d98967399d599d6afeca52fe1c375ec0d`.
- Disposable PostgreSQL and CMS-to-HAL-to-virtual-charge-point topology are required for full acceptance.

## Contract impact

- Adds CPO/App/Platform operational read and scoped SSE contracts. REST snapshots remain authoritative; business APIs use CMS capabilities rather than HAL transport.

## Data and migration impact

- Adds migration 33 for durable scoped operational notifications, migration 34
  for nullable tariff assignment metadata, and migration 35 for same-CPO
  GST-to-hub assignment. All are applied on the
  development VPS after validated rollback dumps.

## Current state

- Source repair is in progress for connector-occupancy ownership, session and
  financial reconciliation, wallet reservation accounting, STOP convergence,
  and strict HAL transaction lookup validation. Claimed migration surface is
  `000045_charging_session_occupancy_and_reconciliation`.

- `halops` owns CMS mapping/command mechanics, exact-ID reconciliation, and fact ingress; `halclient` remains the wire adapter.
- `liveops` reads committed CMS projections and centralizes freshness/offline connector semantics.
- CPO/Platform snapshots and customer own-session/customer-safe charger detail use those capabilities.
- Full CustomerCharger list, hub-detail, single-detail, and favorite responses
  now use the same bounded `liveops.GetChargerDetails` overlay. Static CMS
  lifecycle remains a customer-safety gate; compact map markers remain only
  name and coordinates.
- The canonical CPO backend HAL operational capability manual is
  `docs/integrations/cpo-hal-operational-capability-manual.md`; it documents
  current CPO reads/SSE and the future command pattern without adding a CPO
  command endpoint.
- The same canonical manual now provides the junior-oriented safe-extension,
  return-value, identifier, lifecycle, debugging, and verification reference;
  the CPO handoff is intentionally only its orientation and reading path.
- The current CPO integration repair keeps those capabilities intact while
  removing duplicate response declarations and create-path mapping delivery;
  new charger identity compatibility and migration-34 tariff metadata are
  source-aligned. The later serial-number HAL admission contract remains out
  of scope.
- Durable operational events are produced with accepted newer fact projections and consumed through scoped REST cursor replay/SSE. Streams revalidate bearer sessions at heartbeat.
- Connection liveness now has a dedicated CMS horizon, independent of meter
  freshness. Accepted newer HAL Heartbeat facts renew the durable connection
  observation; REST/read-side recovery and realtime invalidation therefore use
  the same source of truth. Connector status is live only while parent
  connection evidence remains fresh.
- Runtime revision `c6b79d4` is active under `evcmsnew-dev.service`; migrations
  through forty-six,
  loopback/public health and readiness, Swagger, raw OpenAPI, the live
  189-operation contract, singular HAL runtime table mappings, and GST-hub
  route boundaries have been verified. No migration was needed for the model
  correction, the User App history release, state-aware GST validation, and
  fresh-availability charging admission.
- Charging-start correction is deployed: mapping is now a pre-command
  prerequisite; an initial mapping failure returns temporary unavailability
  without a commercial record. The post-transaction mapping reconfirmation is
  retained solely for the inventory-change race and atomically terminalizes a
  known unattempted start. Exact HAL command GET 404 now invokes the
  customer-charging transaction that releases only a HELD hold and marks the
  unmaterialized START `REJECTED`/`EXPIRED` plus `CONFIRMED_ABSENT`; lookup
  infrastructure errors and all STOP absence remain reconciliation-required.
- Charging HTTP handlers now take the server-generated `RequestLogger` ID from
  Gin context for HAL mapping/start/stop correlation. A User App
  `X-Request-ID` is neither required nor authoritative; `halclient` rejects an
  empty mutation correlation locally before it can reach HAL.
- Source-only persistence correction: `ChargingSession.TotalKWh` explicitly
  maps to the migration-owned `charging_sessions.total_kwh` column. GORM's
  inferred `total_k_wh` caused the observed PostgreSQL `42703`; no migration,
  live database mutation, or deployment is part of this correction.
- 2026-08-25 active slice: add a CPO ADMIN-only live-session REST snapshot and
  filtered durable replay/SSE path. It must return only materialized ongoing
  session live stats from committed CMS projections, preserve meter/SoC
  freshness, filter the durable event log to `CHARGING_SESSION` invalidations,
  revalidate the session during SSE heartbeats, and retain REST recovery after
  reconnect. No live route may synchronously call HAL or expose customer,
  wallet, tariff, or provider-secret data.
- 2026-08-25 admission repair: current commercial admission must prefer the
  PostgreSQL-enforced current subscription over terminal history, so an old
  expired record cannot strand a User App after a later active renewal. Keep
  the existing narrow gate: only new customer charging starts and recharge
  order creation are blocked by current expired/elapsed state.
- 2026-08-25 session-projection repair: historical CPO session reads returned
  zero-value charger objects despite a connector projection. Resolve a missing
  direct session charger from the connector's CPO-owned charger in one bounded
  query, retain no-fabrication behavior when both relations are absent, and
  verify the actual deployed response after publication.

## Verification

- 2026-08-25 charger-relation fallback repair: focused CPO regression and
  route/OpenAPI checks, `go test ./...`, `go vet ./...`, and `git diff --check`
  pass. Runtime revision `0fd9774` was rebuilt and rehosted without a
  migration or database mutation; service readiness, public routing, Swagger,
  Caddy, binary identity, and startup checks passed. `pwsh` and
  `TEST_DATABASE_URL` remain unavailable.

- 2026-08-25 live-session/admission slice: focused customerauth, CPO, liveops,
  operational-event, and route/OpenAPI checks pass; `go test ./...`, `go vet
  ./...`, and `git diff --check` pass. Runtime revision `aa44e4b` was rebuilt
  and rehosted without a migration or database mutation. Live/readiness,
  Swagger, Caddy, expected unauthenticated live-session access, 209-operation
  OpenAPI, and post-rehost logs were verified. The admission regression is
  database-free and proves a current ACTIVE renewal takes precedence over an
  expired historical row. `pwsh` and `TEST_DATABASE_URL` remain unavailable;
  live-session hardware acceptance is not claimed.

- Focused HAL client, customer charging, halops, and liveops tests pass.
  Disposable PostgreSQL migration/occupancy coverage is present but skipped
  because `TEST_DATABASE_URL` is not configured. Full `go test ./...`, vet,
  OpenAPI/runtime parity, documentation verification, and `git diff --check`
  pass; disposable lifecycle verification remains pending.

- Focused User App and live-projection package tests pass after the batch
  overlay refactor. `go test ./...`, `go vet ./...`, the PowerShell
  documentation verifier, OpenAPI/runtime parity, and `git diff --check` pass
  for this merged source state.
- The integration repair's focused CPO, liveops, customerauth, halops, route/
  OpenAPI, and PowerShell documentation checks pass. `go test -p 1 ./...`,
  `go vet -p 1 ./...`, and `git diff --check` pass; serial Go verification
  avoids an environment paging-file limit observed with parallel test process
  startup. `TEST_DATABASE_URL` is unset, so disposable migration and lifecycle
  verification remains pending.
- Focused route/OpenAPI verification, `go test ./...`, `go vet ./...`, Caddy
  validation, clean build, and `git diff --check` passed. Migration ledger,
  table/index, service, health, Swagger, raw OpenAPI, and HAL fact-ingress
  validation checks passed after rehost.
- The shared full-response User App overlay was rehosted without a migration;
  current service state has zero restarts, public/loopback health and docs are
  healthy, and no post-restart warning was recorded. The PowerShell verifier
  remains unavailable on this Ubuntu host because `pwsh` is not installed.
- Fact/mapping/SSE lifecycle behavior, reconciliation recovery, and dual-service
  E2E remain pending. The earlier development host lacked `pwsh`; the current
  Windows source checkout passed the PowerShell documentation verifier.
- 2026-08-19 active repair: investigate and correct the observed condition in
  which HAL has persisted authoritative `transaction.started` truth but CMS
  returns a retried 500 and leaves no customer session. The repair must retain
  HAL physical truth, return typed deterministic fact errors, preserve fact
  receipt/session/event atomicity, and reconcile authoritative transaction
  lookup without inventing a session on a 404.
- The guarded connection-fact sequence regression requires a disposable
  `TEST_DATABASE_URL`; physical `bd9099` acceptance remains a post-deployment
  procedure and is not claimed by source verification.
- 2026-08-14 connection-liveness hardening: focused config/liveops/customer
  tests, `go test ./...`, `go vet ./...`, and `scripts/verify-docs.ps1` pass.
  The guarded PostgreSQL projection regression skips safely without
  `TEST_DATABASE_URL`.
- 2026-08-19 focused database-free customerauth/halops compilation tests pass.
  New disposable-PostgreSQL coverage is present for the charging-start
  prerequisite, ambiguous delivery, exact 404 recovery, fact race, expiry,
  idempotency, and fresh retry but is skipped while `TEST_DATABASE_URL` is
  unset. `go test -p 1 ./...`, `go vet ./...`, focused route/OpenAPI parity,
  `git diff --check`, and `scripts/verify-docs.ps1` pass after reconciling the
  verifier baseline to the authoritative 187-operation schema.
- 2026-08-19 correlation correction deployment: runtime revision `d7f72cd` is
  active with no new migration; local/public readiness, Swagger, raw OpenAPI,
  Caddy validation, and post-rehost logs passed. `pwsh` and
  `TEST_DATABASE_URL` remain unavailable on this host.
- 2026-08-19 request-correlation correction: focused customerauth handler,
  middleware, and halclient tests prove CMS-generated IDs reach Start/Stop,
  client IDs are not trusted, and empty mutation correlation sends no HTTP.
- 2026-08-20 confirmed fresh cross-service incident: HAL emitted Go-named
  command fields while CMS expected snake_case. The 2xx decoder accepted the
  empty semantic result and persisted `uuid.Nil`; this slice makes malformed
  command responses fail closed and lets authoritative start evidence repair a
  stored zero UUID as unknown rather than conflict.
- 2026-08-20 response-boundary repair verification: focused HAL-client/charging
  tests, full `go test ./...`, `go vet ./...`, and `git diff --check` pass. The
  integration recovery cases require a disposable `TEST_DATABASE_URL` and
  were skipped safely. Runtime revision `1729b41` is active after rehost with
  no migration or database mutation; loopback/public readiness, Swagger, raw
  OpenAPI (188 operations), Caddy validation, and the post-rehost journal scan
  passed. `pwsh` remains unavailable on this host.
- 2026-08-20 model-mapping correction: focused and full Go tests, vet, and
  `git diff --check` verify the audited materially acronym-sensitive fields and
  a PostgreSQL-dialect dry-run ChargingSession insert includes `total_kwh`,
  never `total_k_wh`. No migration or database mutation was required. Runtime
  revision `765cc80` is active after rehost; local/public readiness, Swagger,
  raw OpenAPI (188 operations), Caddy validation, and the post-rehost journal
  scan passed.
- 2026-08-20 charging lifecycle repair deployment: migration 45 was backed up
  and applied successfully. Runtime revision `765cc80` is active with the
  occupancy indexes and reconciliation status constraint present; focused and
  full Go verification, local/public readiness, Swagger, raw OpenAPI (188
  operations), Caddy validation, and the post-rehost journal scan passed.
  Disposable PostgreSQL occupancy tests remain skipped without
  `TEST_DATABASE_URL`; `pwsh` is unavailable on this host.
- 2026-08-21 CMS/HAL fact-delivery closure deployment: runtime revision
  `c6b79d4` is active with the authenticated exact-fact requeue route, signup
  challenge replacement, and 189-operation OpenAPI contract. Focused/full Go
  verification, local/public readiness, Swagger, Caddy validation, and the
  post-rehost journal scan passed. PostgreSQL signup races remain skipped
  without `TEST_DATABASE_URL`; `pwsh` is unavailable on this host.
- 2026-08-20 auth/readiness deployment: migration 46 was backed up and applied
  successfully. Runtime revision `166b0de` is active with current-worker
  readiness projection, challenge uniqueness indexes, and non-null GST state;
  focused/full Go verification, local/public readiness, Swagger, raw OpenAPI
  (188 operations), Caddy validation, and the post-rehost journal scan passed.
  PostgreSQL concurrency tests remain skipped without `TEST_DATABASE_URL`, and
  `pwsh` is unavailable on this host.

## Handoff

- Do not mark complete until lifecycle/mapping/fact/SSE behavior and topology acceptance are proven.

## Completion

Not complete.
