# WI-20260807-cpo-network-pricing-operations

Status: In Progress
Owner: Abhranil Pal
Collaborators: None
Started: 2026-08-07
Last updated: 2026-08-07

Development-plan reference:

- `docs/DEVELOPMENT_PLAN.md` — CPO administration and network configuration

Detailed-plan reference:

- `docs/plans/cpo-admin-network-configuration.md`

Issue/PR reference: None

## Outcome

Abhranil Pal owns the active CPO network, pricing, and integration-credential
backend work.

## Scope

- CPO hub, charger, GST, and tariff management
- CPO Razorpay integration-credential management
- CPO-owned static charger status and publication control
- Related CPO agent handoff, API contracts, and documentation

## Non-goals

- This record does not authorize a production deployment, DNS change, database
  mutation, credential disclosure, or external-provider change by itself.
- It does not assign ownership of platform CPO control, CPO/user
  authentication, customer-facing APIs, or HAL integration. Those remain owned
  by their respective work items.

## Claimed surfaces

- CPO network, pricing, and integration-credential routes, handlers, OpenAPI,
  and CPO agent handoff contracts
- `docs/CPO_ADMINISTRATION.md` and related CPO workflow/contract documentation

## Verification

- Documentation verification and whitespace checks passed.