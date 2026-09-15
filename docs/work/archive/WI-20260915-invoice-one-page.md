# WI-20260915-invoice-one-page

Status: Deployed
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
delivery or API changes. Deployment used source revision `d2e5393` after the
user's explicit rehost authorization.

Focused layout tests pass for wide/tall/no logos, long Unicode, rounding, complete
source-text retention, page padding, absence of text overlap and V2 compatibility.
Actual PDF parsing confirms single-page output, A4 for ordinary cases, and all
90 note lines plus the final marker on the long page. Wide/tall logo samples were
visually inspected. In this release audit, focused/full Go tests, vet, module
verification, production build, route parity, and diff checks passed. `pwsh` and
`TEST_DATABASE_URL` are unavailable; the PowerShell docs verifier was not
independently rerun and database lifecycle tests remain unverified.

## Deployment verification

Active binary SHA-256 is
`6b7e076549b13e503f8eab5c90f01106b8a330421842ea14aef5f8023b6190c5`; the
preceding executable is retained at
`/root/evcmsnew-backups/pre-invoice-v3-d2e5393-20260915T103514+0530/evcmsnew`
(SHA-256
`6d44079e64e788a7a12c920999829d047982d3723230d7bbb4d11c3d852835fa`).
Service PID 177170 is active with zero restarts and matching process/install
hashes. Local/public liveness, readiness, docs, and OpenAPI returned 200; all
six required current workers are healthy/fresh; Caddy validates; no new-process
error/panic/fatal entries were found. Migration 000069 remains current. The API
has 245 operations. Aggregates are 77 READY invoices, seven already-SENT
invoice deliveries, and 474 already-SENT mail-outbox jobs. No READY artifact or
mail was changed, sent, or retried. SMTP acceptance remains unverified.
