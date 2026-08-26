# WI-20260826-customer-selected-session-limits

Status: Implemented
Owner: Codex
Collaborators: Anubhab Dey (product and CMS/HAL boundary owner)
Started: 2026-08-26
Last updated: 2026-08-26

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — Charging lifecycle and HAL integration
Detailed-plan reference: `docs/integrations/ocpp-hal-boundary.md`
Issue/PR reference: None

## Outcome

Let an authenticated CPO-local customer choose one durable charging-session
limit: energy, time, or money. Preserve the existing CMS start intent, wallet
hold, HAL v1 start command, HAL stop workflow, fact ingestion, and settlement
vertical as the only execution path.

## Scope

- Add one optional, mutually exclusive customer start-limit request object.
- Persist the chosen limit and the effective HAL Wh/time limits on the existing
  start intent; expose them in customer start-progress and session reads.
- Derive the existing HAL limits and wallet hold without creating a second
  command, worker, or source of truth.
- Extend the HAL v1 contract/store only as needed to retain the source limit
  type through automatic stop evidence.
- Update migrations, OpenAPI, integration/frontend documentation, focused
  tests, and both repositories' project memory.

## Non-goals

- Tariff redesign, arbitrary customer OCPP commands, CPO remote control,
  provider deployment, live database migration, or a claim that a physical
  stop can make a money cap exact to the paise.

## Claimed surfaces

- CMS `src/customerauth`, `src/halops`, `src/halclient`, models, migrations,
  customer OpenAPI/FE/integration docs, tests, and project memory.
- HAL v1 request/store/stop workflow, migration, OpenAPI, tests, and project
  memory.

## Dependencies and blockers

- Existing v1 energy/time stop workflow is the sole protocol enforcement path.
- Meter/stop intervals can overshoot an exact selected amount. The existing
  CPO wallet buffer remains the recovery margin; any settlement beyond its
  held amount remains reconciliation-required instead of silently debiting.
- Disposable PostgreSQL and dual-service/charger acceptance remain required
  for runtime proof.

## Contract impact

- `POST /api/v1/customer/charging-sessions` gains an optional `limit` object.
- The HAL service-only start command gains a validated source limit type; no
  browser or CPO client can call HAL.

## Data and migration impact

- Additive CMS migration 54 preserves existing starts as `AUTO`; it is applied
  in `devevcmsnewdb` after a transactional dry-run and a retained pre-change
  dump at `/root/evcmsnew-backups/devevcmsnew-before-000054-20260826-111224.dump`.

## Current state

- Existing tariffs bill by energy, time, or fixed session. The User App could
  not select a limit: it supplied only charger and connector, while CMS always
  derived a one-hour duration and an energy guard from wallet/capacity.

## Verification

- Focused tests, route/OpenAPI parity, full `go test ./...`, `go vet ./...`,
  and `git diff --check` pass. The binary was rebuilt and rehosted as runtime
  revision `a085f29`; service/readiness, public routing, Swagger/OpenAPI,
  Caddy, and post-rehost startup checks pass. Physical charger and disposable
  PostgreSQL/dual-service acceptance remain pending.

## Handoff

- Preserve explicit separation between pricing basis and customer stop limit.
  Do not convert this into a parallel charging lifecycle or claim exact money
  enforcement after an OCPP stop is requested.

## Completion

Implemented; hardware/disposable-database acceptance remains pending.
