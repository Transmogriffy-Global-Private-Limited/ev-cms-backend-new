# WI-20260915-invoice-tariff-basis

Status: Deployed
Owner: Codex

## Outcome and scope

Correct the current invoice Basis column using frozen tariff billing semantics
and actual usage. Energy bills kWh, time bills elapsed minutes, sessions bills
once. AUTO/ENERGY/TIME/MONEY select independent stop limits, not billing units.
Scope: invoice projection, V3 rendering, tests and invoice documentation.

## Invariants and verification

Preserve amounts/taxes, stop-limit details, one-page layout, legacy V2 dispatch,
and READY artifacts. No settlement, HAL, schema, machine API route/schema, or
live data changes. The human invoice contract was updated upstream. Verified all twelve
tariff/limit combinations, fractional duration, session nil units, historical
per-kWh/per-Wh snapshots, missing/invalid facts, and actual generated PDFs.

Focused/full Go tests, vet, module verification, production build, route parity
and diff checks passed. Twelve generated PDFs are one-page PDF 1.7 files; tests
assert tariff basis, independent limit text and unchanged monetary rows.
Representative time/session PDFs were visually inspected. Matrix fixtures use
valid admission limits that differ from actual usage, including a MONEY ceiling
above fixed-session cost. `pwsh` is unavailable, so the source item's documentation
verifier result was not independently rechecked here. `TEST_DATABASE_URL` is
unset; database lifecycle tests and SMTP acceptance remain unverified.

## Deployment verification

Source revision `cb4056f` is deployed; active binary SHA-256 is
`4a836493a7fc3197decfeb1d09d29a55f324fb600e306fb79394b447410aff70`. The
previous executable is retained at
`/root/evcmsnew-backups/pre-invoice-tariff-basis-cb4056f-20260915T143959+0530/evcmsnew`
(SHA-256
`6b7e076549b13e503f8eab5c90f01106b8a330421842ea14aef5f8023b6190c5`). Service
PID 190918 is active with zero restarts and matching process/install hashes.
Local/public health, readiness, docs, and OpenAPI returned 200; all six required
current workers are healthy/fresh; Caddy validates; the new-process
error/panic/fatal scan is clear. Migration 000069 remains current, with 245 API
operations. Aggregates are 79 READY invoices, nine already-SENT invoice
deliveries, and 475 already-SENT mail-outbox jobs. No invoice or email was
changed, sent, or retried.
