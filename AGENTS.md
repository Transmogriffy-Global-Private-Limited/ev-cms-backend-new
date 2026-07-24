# AGENTS.md

## Scope

These instructions apply to the entire `ev-cms-backend-new` repository.

## Required Project Memory

Before planning or changing code, read:

- `docs/DEVELOPMENT_PLAN.md`
- `docs/PROJECT_STATE.md`
- `docs/SCHEMA.md`
- `docs/AUTHENTICATION.md`
- `docs/CPO_ADMINISTRATION.md`
- `docs/AI_CHANGELOG.md`
- `docs/README.md`
- relevant educational guides under `docs/guides/`
- relevant integration records under `docs/integrations/`
- relevant contracts under `docs/contracts/`
- relevant records under `docs/decisions/`
- relevant active plans under `docs/plans/`
- `docs/plans/superadmin-control-plane.md` while the complete platform control
  plane is active

Keep those documents synchronized with meaningful implementation and
verification changes.

## Required Documentation Surfaces

- Educational documentation lives under `docs/guides/`.
- External integration boundaries live under `docs/integrations/`.
- Stable HTTP, internal message, and configuration contracts live under
  `docs/contracts/`.
- Realtime contracts live under `docs/contracts/realtime/`; platform SSE uses
  `docs/contracts/realtime/platform-events.md`.
- `docs/AUTHENTICATION.md` and `docs/CPO_ADMINISTRATION.md` remain the detailed
  workflow references. `docs/contracts/api/administrative-http-api.md` is the
  complete human endpoint contract.
- `docs/contracts/api/subscriptions.md` is the granular platform subscription
  and entitlement contract.
- `docs/contracts/api/platform-billing.md` is the granular provider-neutral
  platform billing-record contract.
- `docs/contracts/openapi/openapi.yaml` is the canonical machine-readable HTTP
  contract. It is embedded and served at `/openapi.yaml`; Swagger UI is served
  at `/docs/`.
- `API_DOCS_ENABLED` must control registration of both documentation routes,
  and enabled/disabled behavior must be verified.
- Every route or payload change must update handlers, the human API contract,
  OpenAPI, tests/fixtures, and consumer guidance in one slice.
- No generated API client currently exists. Do not claim an SDK is current
  until generation and drift verification are implemented.
- Run `.\scripts\verify-docs.ps1` after meaningful documentation, route, or
  configuration changes.

## Permanent Boundaries

- The CMS is a multi-tenant CPO management application.
- A CPO is a tenant organization, not a global user role.
- Platform superadmin access and CPO staff access are separate authorization
  planes.
- The OCPP HAL remains a separate service and owns OCPP connections, protocol
  state, exact OCPP transaction identifiers, and raw meter communication.
- The CMS and HAL communicate through authenticated, idempotent service
  contracts. Neither service writes the other service's database.
- Tenant-owned records must carry a trusted CPO identifier.
- Do not add custom RBAC, event infrastructure, caching, or service boundaries
  before a concrete requirement needs them.

## Verification

For every meaningful slice, run:

```powershell
gofmt -w <changed-go-files>
.\scripts\verify-docs.ps1
go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1
go test ./...
go vet ./...
git diff --check
git status --short
```
