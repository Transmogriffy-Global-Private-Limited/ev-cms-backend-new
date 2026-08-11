# WI-20260807-platform-userapp-infra-operations

Status: In Progress
Owner: Anubhab Dey
Collaborators: None
Started: 2026-08-07
Last updated: 2026-08-11

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — Superadmin control plane and customer app
  experience work

Detailed-plan reference:

- `docs/plans/superadmin-control-plane.md`
- `docs/plans/customer-app-experience.md`
- `docs/plans/customer-authentication.md`

Issue/PR reference: None

## Outcome

Anubhab Dey owns the active SuperAdmin and User App backend work, the shared
mail, notification, and authentication infrastructure those surfaces use, and
the related development server hosting, PostgreSQL setup, and DNS operations.
He also owns the HAL-to-CMS and CMS-to-HAL communication changes. Each
respective CMS surface owner owns the reception and handling of messages at
their CMS surface.

## Scope

- Platform SuperAdmin backend and its control-plane contracts
- User App backend and customer-facing HTTP contracts
- Shared authentication, mail outbox/SMTP delivery, and notification behavior
- HAL-to-CMS and CMS-to-HAL service communication contracts and delivery flows
- Development hosting, service configuration, database setup/migrations, and
  DNS work required for those backend surfaces

### Active coordination note (2026-08-11)

Codex is repairing the SuperAdmin administrator-list GORM base-model defect in
`src/superadmin/service.go` and adding focused SQL-shape plus disposable
PostgreSQL regression coverage. This is an internal query correction only: it
does not change the HTTP contract, authorization, schema, migrations, or any
other claimed surface.

Codex is adding a compact authenticated User App charger-location list under
`src/customerauth/`, its route, OpenAPI, and frontend contract. It must reuse
the full charger-list filters and published-hub/current-CPO boundary while
returning only charger name and attached-hub coordinates. No migration or
deployment is included.

## Non-goals

- This record does not authorize a production deployment, DNS change, database
  mutation, credential disclosure, or external-provider change by itself.
- It does not make Anubhab the owner of the HAL's internal OCPP connections,
  protocol state, transaction identifiers, or raw meter communication.
- CMS-side receipt, validation, domain handling, and persistence remain owned
  by the respective CMS surface owner; the communication owner must coordinate
  changes through the authoritative integration contract.

## Claimed surfaces

- `src/auth/`, `src/customerauth/`, `src/mail/`, and notification-related code
- Platform routes, handlers, OpenAPI, and SuperAdmin integration contracts
- User App routes, handlers, OpenAPI, and User App integration contracts
- CMS/HAL authenticated, idempotent service contracts and related integration
  documentation
- PostgreSQL migrations and environment/configuration needed by these surfaces
- `docs/guides/operations/`, relevant integrations, and deployment/DNS records

## Dependencies and blockers

None currently recorded.

## Contract impact

The User App published-charger projection now includes safe CPO-owned display,
category, use, and parking metadata. `CustomerHub.open_24_hours` is distinct
from `CustomerCharger.twenty_four_seven_open_status`; the latter is sourced
from the charger, and `hub_open_24_hours` identifies the attached hub value.
The projection continues to omit charger-host contact details, connection URLs,
sanctioned load, and HAL-owned state. Any concrete contract change must be
recorded and updated in its canonical contract, OpenAPI or service contract,
consumer guidance, and verification in the same implementation slice. The
communication change owner and receiving CMS surface owner must agree on
message semantics, authentication, idempotency, error handling, and
verification before a change is completed.

## Data and migration impact

Potentially affects the CMS PostgreSQL database, migrations, and deployment
configuration. Database and hosting actions require their own scoped review and
verification before execution.

## Current state

The ownership registration remains active. User App charger projections include
the CPO-owned display-safe `charger_name`, category/use, and parking metadata,
alongside safe hub fields and `connector_total_capacity`. Hub
`open_24_hours`, charger `twenty_four_seven_open_status`, and
`hub_open_24_hours` are separately represented. Charger-host contact details,
connection URLs, sanctioned load, and HAL-owned state remain excluded. The
development VPS now runs application revision `d368903`; the charging vertical,
nullable tariff metadata, tenant settings API, GST state update, and protected
invoice-logo retrieval are active through migration thirty-one
without a DNS or reverse-proxy change.

The isolated SuperAdmin administrator-list repair now binds GORM explicitly to
`models.PlatformAdmin` before joining `users`. Its SQL-shape regression test
passes, and a guarded disposable-PostgreSQL test covers execution, authority
filtering, projections, and keyset pagination when `TEST_DATABASE_URL` is set.

The local User App source now includes a compact map-marker projection for
published chargers: `charger_name`, attached-hub `latitude`, and attached-hub
`longitude` only. It shares the full charger list's optional filters, cursor
rules, near-me behavior, current-CPO scope, and customer-visible-hub boundary.

## Verification

- `go test ./src/customerauth -count=1` passed for the compact map-marker
  projection, including serialized exact-field coverage.
- `./scripts/verify-docs.ps1` passed with the OpenAPI count updated to 158.
- The focused route/OpenAPI Go test is currently unverified on this workstation:
  an unrestricted run exhausted available Go runtime memory, and bounded
  single-process retries exceeded two minutes while compiling. No migration,
  deployment, or database action was attempted.
- The database-free User App projection test now proves serialized hub and
  charger opening-hour values remain separate when they differ.
- `go test ./src/customerauth -count=1` and OpenAPI/runtime route-contract
  verification passed for the split.
- `go test ./...`, `go vet ./...`, the opening-hour residue scan, and
  `git diff --check` passed for the split.
- The database-free User App projection test now covers the propagated
  charger, hub, and connector-capacity fields while retaining `UNKNOWN` live
  availability.
- `go test ./src/customerauth -count=1`, OpenAPI/runtime route-contract
  verification, `go test ./...`, `go vet ./...`, and `git diff --check`
  passed.
- OpenAPI/runtime route-contract verification passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- Revision `d368903` is active under `evcmsnew-dev.service`; the running
  process matches the installed binary and the expected VCS revision.
- Loopback/public liveness and readiness passed.
- The live OpenAPI exposes 161 operations, and the live CPO user-group detail
  `members` projection and `usergroup_assigned` response field are present.
- The live CPO connector response schema reflects `connector_total_capacity`.
- The current systemd state is active/enabled with zero restarts after the
  bounded SSE shutdown deadline recovered during rehost.
- Migration thirty-one is applied; the tenant settings, GST, and CMS/HAL charging routes are
  protected and the optional HAL provider remains unconfigured on this host.
- The protected User App charger route returned `401` without credentials.
- The current release is active on the loopback listener and public health and
  Swagger routes passed; the live OpenAPI contains 161 operations.
- The User App documentation remains aligned with its FE handoff and OpenAPI
  `connector_total_capacity` field.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.
- `go test ./src/superadmin -count=1`, `go test -p 1 ./...`, `go vet -p 1
  ./...`, and `git diff --check` passed for the administrator-list repair.
- The PostgreSQL execution regression test is present but skipped locally
  because `TEST_DATABASE_URL` is not configured; no database was selected or
  modified for this repair.

## Handoff

Anubhab Dey is the active owner. Other contributors should consult this item
before changing a claimed responsibility and record any genuine overlap here.

## Completion

Not complete. Move this item to `docs/work/archive/` after the ownership scope
is released or superseded, with final outcome and verification recorded.
