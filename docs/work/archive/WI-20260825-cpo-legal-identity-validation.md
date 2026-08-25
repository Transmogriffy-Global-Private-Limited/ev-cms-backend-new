# WI-20260825-cpo-legal-identity-validation

Status: Completed; source verified
Owner: Codex
Collaborators: Anubhab Dey
Started: 2026-08-25
Last updated: 2026-08-25

Development-plan reference: `docs/DEVELOPMENT_PLAN.md` — CPO administrator and initial network configuration
Detailed-plan reference: `docs/decisions/0015-cpo-legal-identity-validation.md`
Issue/PR reference: None

## Outcome

CPO provisioning and profile updates now reject malformed legal-identity and
administrator input before persistence while preserving normalized global GSTIN
uniqueness under concurrent requests.

## Scope

- Full GSTIN structural, checksum, and state-code validation against CPO state.
- Canonical CPO/admin text validation and six-digit Indian PIN-code validation.
- Additive PostgreSQL GSTIN/state/PIN constraints with a fail-closed preflight.
- Updated OpenAPI, human contracts, tests, ADR, and project memory.

## Non-goals

- GST registry lookup or legal-name ownership verification.
- CPO administrative access changes.
- Changes to existing global GSTIN/email uniqueness.

## Claimed surfaces

- `src/cpo`, CPO migrations/tests, shared test fixtures, OpenAPI/contracts,
  project docs, and `AGENTS.md`.

## Dependencies and blockers

- A GSTIN can be structurally validated locally but business-name ownership
  needs an authorized registry integration.
- PostgreSQL migration execution requires a disposable `TEST_DATABASE_URL`.

## Contract impact

- Create/profile APIs now return field-specific `invalid_gstin`,
  `invalid_gstin_state_mismatch`, and `invalid_pincode` errors as applicable.

## Data and migration impact

- Forward migration 53 preflights CPO records and rejects invalid direct writes.
- No database was migrated or mutated in this workspace.

## Current state

- Complete in source. `uq_cpos_gstin_normalized` remains the sole GSTIN
  uniqueness guard; `(gstin, business_name)` would be redundant.

## Verification

- Passed: `./scripts/verify-docs.ps1`, focused CPO and route/OpenAPI tests,
  `go test ./...`, `go vet ./...`, and `git diff --check`.
- Not run: migration application and PostgreSQL direct-write constraints because
  `TEST_DATABASE_URL` is unset.

## Handoff

- Do not add a redundant `(gstin, business_name)` unique index.
- Do not claim business-name ownership verification without an authorized GST
  registry integration.

## Completion

Source and documentation complete; database execution is intentionally pending
a disposable database.
