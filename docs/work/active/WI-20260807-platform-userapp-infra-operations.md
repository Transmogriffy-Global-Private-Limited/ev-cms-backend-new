# WI-20260807-platform-userapp-infra-operations

Status: In Progress
Owner: Anubhab Dey
Collaborators: None
Started: 2026-08-07
Last updated: 2026-08-07

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

Potentially affects platform and User App HTTP/authentication contracts,
mail/notification delivery behavior, HAL service communication contracts, and
hosting configuration. Any concrete contract change must be recorded and
updated in its canonical contract, OpenAPI or service contract, consumer
guidance, and verification in the same implementation slice. The communication
change owner and receiving CMS surface owner must agree on message semantics,
authentication, idempotency, error handling, and verification before a change
is completed.

## Data and migration impact

Potentially affects the CMS PostgreSQL database, migrations, and deployment
configuration. Database and hosting actions require their own scoped review and
verification before execution.

## Current state

The ownership registration remains active. The User App discovery
descriptions use the established `connector_total_capacity` response field.
The development VPS now runs application revision `86170d3`; this deployment
introduced no new migration or DNS/configuration change and reconciles the CPO
connector response contract with the runtime projection while preserving the
existing User App contract.

## Verification

- OpenAPI/runtime route-contract verification passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- Revision `86170d3` is active under `evcmsnew-dev.service`; the running
  process matches the installed binary SHA-256 and embeds the expected clean
  VCS revision.
- Loopback/public liveness and readiness passed.
- The live OpenAPI exposes 137 operations, and the live CPO connector
  response schema reflects `connector_total_capacity`.
- The post-start fatal-error scan passed.
- No migration file changed from the previously deployed revision.
- The User App documentation remains aligned with its FE handoff and OpenAPI
  `connector_total_capacity` field.
- The PowerShell documentation verifier was not run because `pwsh` is
  unavailable on this Ubuntu host.

## Handoff

Anubhab Dey is the active owner. Other contributors should consult this item
before changing a claimed responsibility and record any genuine overlap here.

## Completion

Not complete. Move this item to `docs/work/archive/` after the ownership scope
is released or superseded, with final outcome and verification recorded.
