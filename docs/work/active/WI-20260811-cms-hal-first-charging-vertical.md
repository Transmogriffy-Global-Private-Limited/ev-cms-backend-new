# WI-20260811-cms-hal-first-charging-vertical

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (User App and CMS/HAL communication ownership)
Started: 2026-08-11
Last updated: 2026-08-13

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` - Charging lifecycle and HAL integration
Detailed-plan reference: `docs/integrations/ocpp-hal-boundary.md`
Issue/PR reference: None

## Outcome

Implement and verify the first real CMS consumer charging vertical against
`ocpp-hal-go-new` using PostgreSQL and a virtual OCPP charge point.

## Scope

- CMS-owned HAL configuration, client, mapping synchronization, durable
  start-intent/commercial reservation/session projection, HAL-fact receipt,
  settlement, reconciliation, User App start/stop/status, migration, OpenAPI,
  and CMS integration documentation.

## Non-goals

- Any change to `ocpp-hal-go-new`, `OCPPHAL_Go`, or the legacy CMS.
- Physical charger acceptance, customer realtime transport, new CPO-staff
  features, HAL tariff/wallet decisions, or a shared CMS/HAL database.

## Claimed surfaces

- Charging, wallet, customer-app, integration, configuration, migration, route,
  OpenAPI, integration-documentation, and verification surfaces required for
  the CMS consumer vertical.

## Dependencies and blockers

- HAL provider reference `21836e5d98967399d599d6afeca52fe1c375ec0d` and its
  frozen v1 contract.
- Coordinate the overlap with
  `docs/work/active/WI-20260807-platform-userapp-infra-operations.md`.
- Physical devices are explicitly not required; the virtual OCPP charge point
  is the acceptance device.

## Contract impact

- Adds CMS-owned customer HTTP and service-internal fact-receiver operations.
- Consumes, but does not redefine, the HAL v1 HTTP/fact contract.

## Data and migration impact

- Added additive migration `000028_cms_hal_charging_vertical` for durable
  integration, start-intent, hold/settlement, projection, receipt, and command
  state.

## Current state

- The current shared development deployment is clean source revision
  `a9528c4` with migrations through thirty-six and 178 OpenAPI operations.
  The HAL runtime GORM models explicitly map to the singular migration tables;
  no migration was needed for this correction.

- The source contains the first CMS client, fact receiver, durable records,
  customer start/stop/polling routes, and OpenAPI/configuration documentation.
- Migration `000028_cms_hal_charging_vertical` and migration
  `000029_add_tariff_fields` are applied in the development database. Earlier
  revision `3ca2c35` established the 162-operation live contract; migrations
  thirty through thirty-two for tenant settings, GST state, and hub state are
  also applied. The current deployment is runtime revision `a9528c4` as recorded above.
- The optional HAL v1 base URL and both service credentials remain unset on
  this host, so customer charging is intentionally unavailable until the
  independent provider is configured.
- Full virtual-charger, restart, outage, and reconciliation-worker acceptance
  remains incomplete.
- Follow-on work item `WI-20260812-cms-hal-operational-capabilities` owns the
  reusable operations/projection/consumer expansion; this work item retains
  original charging and settlement acceptance ownership.

## Verification

- Passed: focused customer-auth and OpenAPI/runtime route-parity tests,
  `go test ./...`, `go vet ./...`, migration-up execution, live migration/table
  checks, local/public health/readiness, Swagger/OpenAPI, protected charging
  route checks, and `git diff --check`. The bounded SSE shutdown deadline during
  rehost recovered through systemd; the current service is active with zero
  restarts.
- Passed for the latest shared deployment: revision `3ca2c35`, migration
  thirty-two ledger/table checks, loopback/public health and readiness, live
  Swagger/OpenAPI, protected-route boundary, and post-rehost journal scan.
- The later shared deployment is revision `3f3a952`: migration thirty-three is
  applied and the current live OpenAPI surface has 172 operations. Service,
  public/loopback health, Swagger, and raw OpenAPI remain healthy.
- Revision `2e8fdb3` is now active after migration thirty-four; the live
  service remains healthy with 172 OpenAPI operations.
- Earlier revision `e831b32` was active after migration thirty-five; the live service
  remains healthy with 176 OpenAPI operations.
- Revision `0d50c09` is now active after the HAL runtime model correction; the
  service, public/loopback health and readiness, Swagger, raw OpenAPI, and
  post-rehost journal checks passed.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.
- Still required: disposable-PostgreSQL lifecycle tests, real loopback
  CMS-to-HAL-to-virtual-charge-point acceptance, and restart/outage/retry
  reconciliation coverage.

## Handoff

- Do not mark complete until the durable happy path and core duplicate/restart
  behavior are proven without customer-side HAL access or invented OCPP truth.

## Completion

Not complete.
