# AGENTS.md

## Scope

These instructions apply to the entire `ev-cms-backend-new` repository.

## Required Project Memory

Before planning or changing code, read:

- `docs/DEVELOPMENT_PLAN.md`
- `docs/PROJECT_STATE.md`
- `docs/SCHEMA.md`
- `docs/AI_CHANGELOG.md`
- relevant records under `docs/decisions/`

Keep those documents synchronized with meaningful implementation and
verification changes.

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
go test ./...
go vet ./...
git diff --check
git status --short
```
