# WI-20260825-cpo-superadmin-fe-contracts

Status: Verified
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-25
Last updated: 2026-08-25

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — CPO administrator and platform control-plane surfaces
Detailed-plan reference: `docs/plans/cpo-admin-network-configuration.md` and `docs/plans/superadmin-control-plane.md`
Issue/PR reference: None

## Outcome

Created a complete CPO frontend integration handoff, a direct SuperAdmin/CPO
frontend authority comparison, and an explicit manually reviewed authority/risk
classification for every routed SuperAdmin API.

## Scope

- CPO frontend authentication, tenancy, routes, UI workflows, errors, realtime,
  recovery, and API inventory.
- SuperAdmin-vs-CPO authority differences.
- Manual classification of each `/api/v1/platform/*` operation by its actual
  enforced authority and frontend risk category.

## Non-goals

- Changing authorization behavior, adding platform RBAC, or inventing API
  permissions that are not enforced.
- Any database, deployment, or runtime mutation.

## Claimed surfaces

- CPO/SuperAdmin frontend handoffs, API contract documentation, documentation
  index, and project memory.

## Dependencies and blockers

- Current platform authorization is one `PLATFORM` SuperAdmin gate; a finer
  platform permission model would require an approved backend design.

## Contract impact

- Documentation-only: identifies callable authority, required headers, and
  frontend permission/risk treatment without changing routes or payloads.

## Data and migration impact

None.

## Current state

- Core CPO administration and provider-integration routes require ADMIN.
- CPO support and notifications require an active CPO membership plus the
  verified app ID, not ADMIN route middleware.
- Every platform route is enforced by PLATFORM. The per-row matrix categories
  are frontend risk groups, not server-enforced granular permissions.

## Verification

- Manual OpenAPI/matrix inventory comparison: 65 platform plus 12 shared
  administrative-auth operations, with no missing or extra matrix entry.
- `./scripts/verify-docs.ps1`, focused OpenAPI/runtime route parity,
  `go test ./...`, `go vet ./...`, and `git diff --check` pass.
- No database, deployment, or live-service verification was needed or run.

## Handoff

Use `CPO_FRONTEND_INTEGRATION_HANDOFF.md` for the CPO browser client,
`SUPERADMIN_CPO_FRONTEND_BOUNDARY.md` for cross-plane UI rules, and
`contracts/api/superadmin-permission-matrix.md` for the platform surface.
Do not label documentation-only classifications as server-enforced granular
RBAC.

## Completion

Verified and archived on 2026-08-25.
