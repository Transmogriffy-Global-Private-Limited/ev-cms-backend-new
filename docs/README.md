# Documentation Map

This directory is the durable project memory for the EV CMS backend.

## Canonical Sources

| Need | Canonical source |
|---|---|
| Repository operating contract and safe development workflow | `../AGENTS.md` |
| Approved sequence and active work | `DEVELOPMENT_PLAN.md` |
| Active implementation ownership and handoffs | `work/active/` |
| Implemented and verified reality | `PROJECT_STATE.md` |
| Agent-assisted change history | `AI_CHANGELOG.md` |
| Database ownership and model mapping | `SCHEMA.md` |
| Detailed authentication API semantics | `AUTHENTICATION.md` |
| Detailed platform CPO API semantics | `CPO_ADMINISTRATION.md` |
| Complete SuperAdmin frontend integration handoff | `SUPERADMIN_FRONTEND_HANDOFF.md` |
| Manual platform subscription semantics | `contracts/api/manual-subscriptions.md` |
| CPO backend AI-agent orientation and execution | `CPO_BACKEND_AGENT_HANDOFF.md` |
| CPO frontend tariff and Hub GST integration | `CPO_FRONTEND_TARIFF_GST_HANDOFF.md` |
| Complete User App frontend authentication handoff | `USERAPP_FE_HANDOFF.md` |
| Complete superadmin architecture and boundaries | `guides/concepts/superadmin-control-plane.md` |
| Platform realtime event contract | `contracts/realtime/platform-events.md` |
| CMS HAL operational event contract | `contracts/realtime/operational-events.md` |
| HTTP request-log schema and safety boundary | `contracts/internal/http-request-logging.md` |
| Manual CPO access workflow | `guides/workflows/cpo-onboarding.md` |
| Required CPO registration identity decision | `decisions/0010-required-cpo-registration-identity.md` |
| Safe HTTP request observability decision | `decisions/0011-safe-http-request-observability.md` |
| CPO organization read, admin profile, and network configuration | `guides/workflows/cpo-admin-network-configuration.md` |
| CPO live-operations frontend handoff | `CPO_OPERATIONS_LIVE_FE_HANDOFF.md` |
| CPO live-operations frontend constants | `CPO_OPERATIONS_LIVE_CONSTANTS.md` |
| Approved customer-app experience sequence | `plans/customer-app-experience.md` |
| CPO-scoped customer-account decision | `decisions/0013-cpo-scoped-customer-accounts.md` |
| Subscription expiry customer-command admission | `decisions/0014-subscription-expiry-customer-command-admission.md` |
| Architectural decisions | `decisions/` |
| Detailed approved plans | `plans/` |
| Learning and operational workflows | `guides/` |
| External-system boundaries | `integrations/` |
| CMS consumer of the OCPP HAL v1 contract | `integrations/ocpp-hal-boundary.md` |
| Canonical CPO backend HAL/live-charger integration guide for safe junior and feature work | `integrations/cpo-hal-operational-capability-manual.md` |
| Stable external and internal contracts | `contracts/` |
| Machine-readable HTTP source of truth | `contracts/openapi/openapi.yaml` |
| Development VPS hosting and activation | `guides/operations/dev-hosting.md` |

`contracts/api/administrative-http-api.md` is the complete endpoint index and
auth/control-plane handoff, including manual CPO activation and suspension.
`SUPERADMIN_FRONTEND_HANDOFF.md` turns the implemented platform subset into a
screen, state-machine, TypeScript, realtime, error, security, and verification
handoff while explicitly recording blocked and unimplemented FE behavior.
`contracts/openapi/openapi.yaml` is the machine-readable contract
embedded into the service. The app serves it at `/openapi.yaml` and serves
self-contained Swagger UI at `/docs/` only when `API_DOCS_ENABLED=true`.

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
- `contracts/realtime/` records streaming, replay, ordering, and reconnect
  contracts.
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
