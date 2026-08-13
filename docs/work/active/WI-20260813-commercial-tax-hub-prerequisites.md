# WI-20260813-commercial-tax-hub-prerequisites

Status: In Progress
Owner: Codex
Collaborators: None
Started: 2026-08-13
Last updated: 2026-08-13

Development-plan reference: `docs/DEVELOPMENT_PLAN.md`
Detailed-plan reference: None
Issue/PR reference: None

## Outcome

Separate tariff commercial fields from GST, resolve active GST only through a
charger's hub, and prevent hubless chargers from being active or customer-visible.

## Scope

- Tariff model/API GST removal and forward migration.
- Customer price/start tax resolution and frozen tax snapshots.
- Charger publication/activation service and database invariants.

## Non-goals

- HAL contracts, OCPP behavior, wallet threshold configuration, or deployment.

## Claimed surfaces

- `src/models/`, `src/cpo/`, `src/customerauth/`, `db/migrations/`, contracts,
  project documentation.

## Dependencies and blockers

- Migration fails safely if deployed `tariffs.gst_id` values are non-null.

## Contract impact

- Tariff payloads no longer accept or return `gst_id`; hub GST is the tax source.

## Data and migration impact

- Adds migration 38; does not infer or copy legacy tariff GST data.

## Current state

- Core model, resolver, start snapshot, and charger guard work is implemented;
  contract and verification reconciliation remains.

## Verification

- Focused package tests pending final reconciliation.

## Handoff

- Do not publish this work. Preserve the frozen CMS-to-HAL boundary.

## Completion

- Pending.
