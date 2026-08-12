# WI-20260811-cms-hal-first-charging-vertical

Status: In Progress
Owner: Codex
Collaborators: Anubhab Dey (User App and CMS/HAL communication ownership)
Started: 2026-08-11
Last updated: 2026-08-12

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

- The source contains the first CMS client, fact receiver, durable records,
  customer start/stop/polling routes, and OpenAPI/configuration documentation.
- Migration `000028_cms_hal_charging_vertical` and migration
  `000029_add_tariff_fields` are applied in the development database. Revision
  `3ca2c35` is active under `evcmsnew-dev.service` with a
  162-operation live contract; migrations thirty through thirty-two for tenant
  settings, GST state, and hub state are also applied.
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
