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
No repricing, schema/API change, existing READY artifact rewrite, delivery, or
migration. Deployment was later explicitly authorized and is recorded below.
Existing commercial work is read-only.

## Verification

Focused/full Go tests, vet, module verification, OpenAPI parity, all-package
build, and diff checks passed. Ghostscript rasterized normal/overflow PDFs as
one/three pages; the normal PDF was visually inspected. `pwsh` is unavailable,
so the PowerShell docs verifier was not independently run in this environment.
`TEST_DATABASE_URL` is unset; PostgreSQL lifecycle tests and live SMTP
acceptance remain unverified.

## Deployment

After the user's explicit rehost authorization, source revision `2eeb5e0`
(application change `00023f2`) was deployed. The test-only fixture correction
in `2eeb5e0` aligns the sample with 5.250 kWh × INR 20/kWh plus 18% GST; it
does not alter runtime behavior. Active binary SHA-256 is
`6d44079e64e788a7a12c920999829d047982d3723230d7bbb4d11c3d852835fa`; the
immediately preceding executable is retained at
`/root/evcmsnew-backups/pre-invoice-breakdown-2eeb5e0-20260915T042431Z/evcmsnew`
(SHA-256
`a6200e596cc01d8f70cb113ab8518d93930364568652b58e72e6aa58fa35ce5`).

Post-rehost the service was active as PID 171693 with zero restarts and matching
process/install hashes. Local and HTTPS liveness/readiness/docs/OpenAPI returned
200; the contract remained at 245 operations, migration 000069 remained
current, all six required current workers were healthy/fresh, Caddy validated,
and no new-process error/fatal/panic entries were found. Aggregate state was 76
READY invoices, six already-SENT invoice deliveries, and 474 already-SENT
mail-outbox jobs. No invoice artifact changed and this deployment sent or
retried no mail.
