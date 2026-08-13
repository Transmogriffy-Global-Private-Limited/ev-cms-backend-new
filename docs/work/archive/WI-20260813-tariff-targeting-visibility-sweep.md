# WI-20260813-tariff-targeting-visibility-sweep

Status: Implemented locally; publication not requested
Owner: Codex
Collaborators: Abhranil Pal (CPO network/pricing owner); Anubhab Dey (User App owner)
Started: 2026-08-13
Last updated: 2026-08-13

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — Customer access/tariffs and charging lifecycle

Detailed-plan reference:

- `docs/plans/cpo-admin-network-configuration.md`
- `docs/plans/customer-app-experience.md`

Issue/PR reference: None

## Outcome

Correct the tariff single-target data model and make CPO tariff CRUD, User App
price display, and charging-start snapshot selection use the fixed precedence
`USERGROUP > CHARGER > HUB`. Preserve the existing customer charger-publication
gate while closing any remaining User App bypasses.

## Scope

- Forward tariff migration and GORM target mapping
- Scoped CPO tariff request/response, validation, and nested-resource behavior
- Shared User App effective-tariff selection and charging-start use
- Customer charger publication checks for discovery and new actions
- Focused tests and authoritative contracts/documentation

## Non-goals

- HAL/OCPP, legacy CMS, QR APIs, wallet settlement, session ownership, deployment,
  or Git publication
- New visibility concepts or physical charger state changes

## Claimed surfaces

- `db/migrations/000037_*`
- `src/models/`, `src/cpo/`, and `src/customerauth/` tariff/publication paths
- tariff OpenAPI, CPO/User App contracts, plans, project state, changelog

## Dependencies and blockers

- The disposable PostgreSQL lifecycle suite requires an explicitly selected
  `TEST_DATABASE_URL`; no database is currently selected.

## Contract impact

- Tariff scope comes only from the nested CPO route; request bodies contain
  commercial fields only. Responses expose `assigned_to` and exactly the
  matching target ID.
- User App selection is `USERGROUP > CHARGER > HUB`; no client reconstructs it.

## Data and migration impact

- Adds a forward migration that makes all tariff targets nullable individually,
  normalizes legacy rows, and enforces exactly one target plus matching
  `assigned_to`. The guarded down migration refuses data that old hub-required
  schema cannot represent.

## Current state

- Migration thirty-seven, GORM mapping, CPO target-scoped CRUD, shared User App
  selector, and the image-route publication correction are implemented locally.
- Audit covered list, locations, hub preloads, direct detail, price, favorite,
  image, and new-start paths. Session-owned history/detail/stop intentionally
  remain outside publication gating.

## Verification

- Passed: `go test ./...`, `go vet ./...`, focused db/CPO/customer-auth/models
  checks, OpenAPI runtime-route parity, `./scripts/verify-docs.ps1`, and
  `git diff --check`.
- PostgreSQL lifecycle cases are present but skipped because `TEST_DATABASE_URL`
  is unset; no disposable database was selected. No deployment occurred.

## Handoff

- This work overlaps the active CPO network/pricing and User App records by
  user-authorized domain correction. Preserve their unrelated active work and
  record any newly discovered boundary here.

## Completion

- Scoped handoff is complete. Archive this item after final residue/status
  confirmation; no commit or publication is authorized by this work item.
