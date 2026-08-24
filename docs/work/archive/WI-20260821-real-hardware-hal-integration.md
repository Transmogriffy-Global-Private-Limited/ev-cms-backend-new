# WI-20260821-real-hardware-hal-integration

Status: Implemented
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-21
Last updated: 2026-08-21

Development-plan reference: `docs/DEVELOPMENT_PLAN.md`
Detailed-plan reference: `docs/integrations/ocpp-hal-boundary.md`
Issue/PR reference: None

## Outcome

Make CMS mapping, start eligibility, physical charger identity, and HAL
configuration reconciliation coherent for real OCPP chargers without changing
the CMS/HAL ownership boundary.

## Scope

- UUID mutation correlation and actionable safe mapping diagnostics.
- Fresh Preparing start eligibility separate from occupancy projection.
- Optional serial mapping contract with the HAL.
- Cross-repository tests, migrations, contracts, and operational guidance.

## Non-goals

- Live database/VPS/Caddy changes, tariff/wallet redesign, or legacy HAL edits.

## Claimed surfaces

- `src/halclient`, `src/halops`, `src/liveops`, `src/customerauth`, models,
  migrations, contracts, tests, configuration, and project memory.

## Dependencies and blockers

- HAL companion work item owns OCPP ingress and configuration reconciliation.
- Disposable PostgreSQL and real hardware acceptance remain external checks.

## Contract impact

- Mapping mutation correlation is a canonical UUID; mapping gains optional
  charger serial evidence.

## Data and migration impact

- Additive only; existing mappings remain valid without serial/diagnostics.

## Current state

Phase-0 evidence confirmed a non-UUID reconciliation label reached HAL and was
rejected before mapping persistence. CMS now uses canonical mutation UUIDs,
safe mapping diagnostics, optional serial evidence, and separate Preparing
start eligibility without changing the display projection.

## Verification

Focused package tests, broad Go package tests, vet, docs verification, and a
residue scan passed. Database lifecycle tests require `TEST_DATABASE_URL`.

## Handoff

Do not make HAL UUID validation permissive and do not turn Preparing into a
globally Available projection.

## Completion

Source implementation is complete. Migration application and physical
CMS-to-HAL-to-charger acceptance remain external and unrun.
