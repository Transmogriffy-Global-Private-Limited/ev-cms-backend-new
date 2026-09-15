# WI-20260915-invoice-breakdown

Status: Complete
Owner: Codex
Started: 2026-09-15

## Outcome and scope

Correct Charges header vertical centering and populate net/session and GST
amounts in `src/invoice`, with focused regressions and documentation.

## Decisions and boundaries

Use visible glyph bounds with Canvas's actual coordinate translation. Allocate
the frozen GST-inclusive settled total by validated frozen rates; round displayed
components to two decimals and expose any rounding residual. Missing/invalid
tax snapshots must not silently become zero tax. Zero configured rates are valid.
No repricing, schema/API change, existing READY artifact rewrite, delivery,
deployment or migration. Commit/push to both branches authorized by the user. Existing commercial work is read-only.

## Verification

Focused and full Go tests plus vet pass. Normal/overflow PDFs open and rasterize
as one/three pages; normal PDF visually inspected for header centering and tax
amounts. Documentation verification passes. TEST_DATABASE_URL is unset; PostgreSQL
lifecycle tests and live generation/delivery remain unverified.
