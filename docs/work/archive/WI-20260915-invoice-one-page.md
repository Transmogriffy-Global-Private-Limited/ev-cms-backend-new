# WI-20260915-invoice-one-page

Status: Complete
Owner: Codex
Started: 2026-09-15

## Outcome and claimed surfaces

Professional, single-page invoice with padded, aspect-preserving logo placement,
clear invoice identity, session summary and charge hierarchy. Scope: versioned
renderer under `src/invoice`, focused tests, canonical invoice guidance and project
documentation. Retain V2/V1 dispatch for already-stamped invoice work.

## Constraints and verification

Keep all text, frozen amounts, rates, identity and references complete; preserve
existing READY artifacts. User explicitly requested no information loss. Normal
output is A4; exceptional long text extends the same page vertically. No truncation
or font shrinking; source text wraps at readable sizes. No billing, snapshot schema,
delivery or API changes. No deployment.

Focused layout tests pass for wide/tall/no logos, long Unicode, rounding, complete
source-text retention, page padding, absence of text overlap and V2 compatibility.
Actual PDF parsing confirms single-page output, A4 for ordinary cases, and all
90 note lines plus the final marker on the long page. Wide/tall logo samples were
visually inspected. Full Go tests, vet, documentation verification and diff checks
pass. `TEST_DATABASE_URL` is unset; database lifecycle checks and live delivery are
unverified. Local commit and branch synchronization are authorized; this slice is not deployed.
