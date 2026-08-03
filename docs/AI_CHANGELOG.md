# AI Changelog

## 2026-08-03

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
