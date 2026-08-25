# Project Operating Contract

## Project Identity

`ev-cms-backend-new` is a Go 1.25 multi-tenant CMS for charge-point operators
(CPOs). It owns the commercial and administrative management plane of an EV
charging network. PostgreSQL is the durable source of truth. The separate OCPP
HAL owns charger connections, OCPP protocol state, exact OCPP transaction
identifiers, and raw meter communication.

The CMS and HAL communicate only through authenticated, idempotent service
contracts. Neither service writes the other's database or absorbs the other's
protocol responsibilities.

## Core Engineering Rules

- Own the requested behaviour end to end: trace caller, validation,
  authentication, authorization, durable transition, side effects, consumers,
  retry/duplicate handling, recovery, contracts, and verification.
- Make the smallest coherent change. Preserve working contracts and do not add
  speculative services, custom RBAC, event infrastructure, caching, or abstractions.
- Prefer explicit application orchestration and PostgreSQL-enforced invariants
  over hidden state or database-owned business flows.
- Never expose secrets, personal data, credentials, OTPs, tokens, raw request
  paths, query strings, request/response bodies, app IDs, API messages, or
  panic values in code, documentation, or logs.
- Do not commit, push, deploy, restart/reload services, change DNS, change
  public exposure, modify live data, run migrations against a live database,
  or make destructive system/database changes without explicit human approval.

## Architecture and Ownership

- `main.go` composes configuration, PostgreSQL, migrations, bootstrap,
  security, services, workers, and the Gin router. It starts the mail and
  platform workers and performs graceful shutdown.
- `src/routes/` owns HTTP assembly, health checks, middleware ordering, and
  environment-controlled OpenAPI/Swagger registration.
- `src/auth/` owns platform/CPO administrative identity, sessions, OTP, and
  credential lifecycle. `src/customerauth/` separately owns CPO-local app-user
  identity, sessions, discovery, wallet, and favourites.
- `src/cpo/` owns CPO organization, CPO ADMIN, network, GST, and tariff
  behaviour. `src/superadmin/`, `src/platformops/`, and `src/subscriptions/`
  own their distinct platform-control concerns. `src/integrations/` owns
  encrypted CPO integration credentials. `src/mail/` owns the durable mail
  outbox/worker. `src/security/` owns crypto primitives and `src/models/` the
  persistence mapping.
- `db/migrations/` contains immutable versioned PostgreSQL migrations;
  `cmd/migrate/` is the explicit migration command. Do not edit an applied
  migration—add a forward migration.
- Tenant state must use a trusted CPO identifier derived server-side. Never
  trust a client-provided CPO/tenant ID without scope validation.
- Platform SuperAdmin and CPO staff are separate authorization planes. A CPO
  is a tenant organization, not a global role. Current callable tenant
  administrative authority is `ADMIN`; `OWNER`, `OPERATOR`, and `VIEWER`
  are dormant schema capacity.

## Runtime, Configuration, and Operations

- Run locally with `go run .`; the default listener is `127.0.0.1:8080`.
  Configuration loads from ignored `.env`, then process environment overrides.
  Copy `.env.example`; never commit `.env` or real credentials.
- `docs/contracts/configuration.md` is the configuration reference. In
  particular, `API_DOCS_ENABLED` controls registration of both `/docs/` and
  `/openapi.yaml`; `LOG_LEVEL=DEBUG` never relaxes logging exclusions.
- The service applies pending up migrations and idempotently bootstraps the
  configured initial superadmin at startup. Use
  `go run ./cmd/migrate -direction up|down` only after inspecting the target
  database; down migrations can remove data.
- The development hosting procedure, health checks, and safe rehost sequence
  are in `docs/guides/operations/dev-hosting.md`. Inspect the actual unit,
  listener, proxy, environment, logs, and rollback path before any operational
  action. Keep application listeners loopback-bound unless deliberate public
  exposure is approved.

## Contracts, Security, and Recovery

- `docs/contracts/openapi/openapi.yaml` is the canonical machine-readable
  HTTP contract. It is embedded and served at `/openapi.yaml`; Swagger UI is
  served at `/docs/` only when `API_DOCS_ENABLED=true`.
- `docs/contracts/api/administrative-http-api.md` is the complete human HTTP
  contract. Every route or payload change updates handlers, OpenAPI, human
  contract, tests/fixtures, consumer guidance, and enabled/disabled docs-route
  verification in the same slice.
- No generated API client exists. Do not claim an SDK is current until
  generation and contract-drift verification are implemented.
- Realtime uses `docs/contracts/realtime/platform-events.md`; it is not
  durable truth. Consumers must retain the documented REST catch-up/recovery path.
- `docs/contracts/internal/http-request-logging.md` owns the safe JSON log
  schema, request ID, proxy trust, developer logging rules, and exclusions.
- CPO activation/suspension is a manual SuperAdmin decision. Subscription
  status never grants or revokes CPO administrative access. A CPO subscription
  that is explicitly `EXPIRED` or has reached `current_period_ends_at` blocks
  only new customer charging starts and new customer wallet-recharge orders;
  keep stop, reconciliation, settlement, read, and pre-expiry payment
  verification paths available. Do not add feature keys or entitlement
  overrides without an approved module catalog and enforcement design. Platform
  invoices, payments, checkout, and provider webhooks require a new explicit
  decision beyond ADR 0012.
- CPO registration identity is normalized and durable: GSTIN is globally
  unique, checksum-valid, and state-matched; pincode is a six-digit Indian PIN.
  Do not add a redundant `(gstin, business_name)` unique key or claim legal-name
  ownership verification without an authorized registry integration (ADR 0015).
- Preserve transaction boundaries, database constraints, idempotency, and
  durable outbox semantics. New asynchronous or cross-service behaviour must
  document ownership, retry layer, duplicate handling, terminal failure, and
  reconciliation before completion.

## Required Project Memory and Documentation

Before any meaningful change, read:

- `README.md` and `docs/README.md`
- `docs/DEVELOPMENT_PLAN.md`, `docs/PROJECT_STATE.md`, and
  `docs/AI_CHANGELOG.md`
- `docs/SCHEMA.md`, `docs/AUTHENTICATION.md`, and
  `docs/CPO_ADMINISTRATION.md` when their surfaces are relevant
- relevant guides, integrations, contracts, ADRs, and detailed plans under
  `docs/`
- `docs/SUPERADMIN_FRONTEND_HANDOFF.md` for every SuperAdmin frontend or
  frontend-integration task; it must distinguish callable, blocked, planned,
  and intentionally unsupported behaviour
- `docs/CPO_BACKEND_AGENT_HANDOFF.md` for every CPO-side backend task
- `docs/plans/superadmin-control-plane.md`,
  `docs/plans/manual-platform-subscriptions.md`, and
  `docs/plans/cpo-admin-network-configuration.md` while their work is active
- `docs/work/active/` and any overlapping work item before changing a shared
  responsibility

`docs/README.md` is the documentation index. Keep the canonical contract,
educational/integration guidance, plan, project state, changelog, and any
relevant ADR synchronized with actual behaviour. Do not claim verification that
was not run. Run `./scripts/verify-docs.ps1` after meaningful documentation,
route, or configuration changes (use PowerShell where it is available; report
its absence separately).

## Coordination and Change Safety

- `docs/work/active/` is the canonical live-work ledger. Create or update a
  narrowly scoped work item for meaningful work, list claimed surfaces, and
  coordinate genuine overlap rather than independently changing the same
  contract, invariant, or migration. Move completed items to `archive/`.
- Inspect `git status --short` before editing. Preserve unrelated user changes.
  Use explicit file lists for staging only when a commit is explicitly
  requested; never use broad staging by default.
- Before a contract change, search the entire affected class of producers,
  consumers, tests, fixtures, scripts, generated artifacts, and docs. Do not
  fix one stale caller while leaving equivalent callers behind.
- For database work, inspect the target, filters, affected state, and rollback
  route before modifying SQL. Use additive migrations and do not treat a
  database as disposable without explicit confirmation.

## Verification and Definition of Done

Run focused checks first, then the broadest applicable checks from the
repository root:

```bash
gofmt -w <changed-go-files>
./scripts/verify-docs.ps1
go test ./src/routes -run TestOpenAPIContractMatchesRuntimeRoutesAndServesUI -count=1
go test ./...
go vet ./...
git diff --check
git status --short
```

Use `./scripts/verify-docs.ps1` only in a PowerShell host; on this Ubuntu VPS
it may require `pwsh`, which is not assumed available. Database lifecycle
tests require an explicitly selected disposable `TEST_DATABASE_URL`; do not
point them at a live database. State any skipped check, why it was skipped, and
the remaining unverified boundary.

A meaningful slice is complete only when the requested end-to-end behaviour,
tenant/security boundaries, durable state, failure/retry/recovery semantics,
contracts and consumers, focused and broad verification, residue scan,
documentation, and work-item state all agree. Nothing is committed, pushed,
deployed, or operationally changed unless explicitly authorized.
