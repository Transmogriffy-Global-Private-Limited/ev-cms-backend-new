# Documentation Map

This directory is the durable project memory for the EV CMS backend.

## Canonical Sources

| Need | Canonical source |
|---|---|
| Approved sequence and active work | `DEVELOPMENT_PLAN.md` |
| Implemented and verified reality | `PROJECT_STATE.md` |
| Agent-assisted change history | `AI_CHANGELOG.md` |
| Database ownership and model mapping | `SCHEMA.md` |
| Detailed authentication API semantics | `AUTHENTICATION.md` |
| Detailed platform CPO API semantics | `CPO_ADMINISTRATION.md` |
| Architectural decisions | `decisions/` |
| Detailed approved plans | `plans/` |
| Learning and operational workflows | `guides/` |
| External-system boundaries | `integrations/` |
| Stable external and internal contracts | `contracts/` |
| Machine-readable HTTP source of truth | `contracts/openapi/openapi.yaml` |
| Development VPS hosting and activation | `guides/operations/dev-hosting.md` |

`contracts/api/administrative-http-api.md` is the complete human endpoint
handoff. `contracts/openapi/openapi.yaml` is the machine-readable contract
embedded into the service. The app serves it at `/openapi.yaml` and serves
self-contained Swagger UI at `/docs/`.

The route verification test requires every implemented business/health
method-path pair to exist in OpenAPI and rejects advertised operations absent
from Gin. No generated SDK is currently committed.

## Documentation Classes

- `guides/concepts/` explains the mental model and invariants.
- `guides/workflows/` explains complete operator and user journeys.
- `guides/troubleshooting/` starts from symptoms and gives safe checks.
- `integrations/` records ownership, credentials, failure handling, and the
  actual implementation status for every external boundary.
- `contracts/api/` records externally callable HTTP behavior.
- `contracts/internal/` records durable internal message and worker contracts.
- `contracts/configuration.md` records environment configuration.
- `contracts/openapi/` owns the embedded OpenAPI source and serving adapter.

## Verification

Run from the repository root:

```powershell
.\scripts\verify-docs.ps1
go test ./...
go vet ./...
git diff --check
```

The documentation verifier checks required files, route coverage, and removed
configuration names. Go route tests parse and semantically validate OpenAPI,
compare it to runtime routes, and smoke-test `/docs/` and `/openapi.yaml`. These
checks do not prove live SMTP or future HAL behavior.
